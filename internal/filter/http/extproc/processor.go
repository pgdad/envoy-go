package extproc

// processor.go — ADR-0171 header-mode state machine + bidi-stream lifecycle
// (LANDS AT TASK 7 of phase 19.1 IMPL).
//
// Per SPEC §6.8 + ADR-0171 §Decision header-mode portion: this file lands
//
//   - the `stage` enum (request_headers, response_headers; body/trailer
//     stages reserved for 19.2)
//   - the `action` enum (continue, stop, error, immediate-response,
//     continue-but-still-waiting)
//   - `resolveProcessingMode` — parses + validates an *extprocv3.ProcessingMode
//     into a *resolvedProcessingMode per parent §5.P9 (DEFAULT → SEND for
//     headers; DEFAULT → SKIP for trailers; body-modes != NONE PARSE-REJECT
//     in 19.1; trailer-modes != SKIP PARSE-REJECT permanently)
//   - `(*filter).openProcessorStream` — opens the bidi-stream against the
//     configured *grpcclient.ProcessorClient per ADR-0169 + SPEC §6.8
//   - `(*filter).dispatchStage` — async per-stage Send/Recv goroutine with
//     per-message timeout per ADR-0169 §Decision + D6 cancel-and-rebuild
//   - `(*filter).completeStage` — calls applyProcessingResponse synchronously
//     inside the dispatch goroutine; signals ContinueDecoding/ContinueEncoding
//     on the resume channel per ADR-0171 §Decision
//   - `(*filter).handleOverrideMessageTimeout` — at-most-ONCE-per-stage
//     override_message_timeout handler per parent §5.P10 + ADR-0171 §Decision
//   - `(*filter).OnDestroy` — sync.Once-guarded streamCancel + CloseSend per
//     D9 race discipline
//
// `applyProcessingResponse` is a TEMPORARY STUB at this task (Task 7) — the
// real per-stage CommonResponse / ImmediateResponse / mode_override dispatch
// lands at Task 8 in `check.go`. The stub is declared as a function variable
// (rather than a plain function) so Group 7 tests can OVERRIDE the stub to
// drive completeStage / mode_override / override_message_timeout assertions
// without dragging in the Task 8 surface. Task 8 RETIRES the stub and
// publishes the real `applyProcessingResponse` in `check.go`.
//
// **D9 race discipline (NO per-stream mutex)** per ADR-0171 §Decision: the
// framework's sequential decode→encode dispatch invariant + the bidi-stream's
// single-in-flight-message correlation rule together guarantee that at most
// ONE dispatch goroutine is live at any time. The gRPC ClientStream Send-vs-
// Recv concurrent-safety contract (per the gRPC documentation) covers the
// goroutine's own Send + Recv sequencing. OnDestroy uses sync.Once to make
// the streamCancel + CloseSend pair idempotent; the existing f.mu / f.done
// guard (per the Task 2 skeleton + Task 12 race tests) covers the resume-
// after-OnDestroy race surface.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ---------------------------------------------------------------------------
// stage — per-direction per-message-type discriminator per ADR-0171.
// ---------------------------------------------------------------------------

// stage enumerates the per-direction stages the filter dispatches. Phase
// 19.1 activated ONLY the two header stages; the phase-19.2 ADR-0171
// §Decision AMENDMENT (Task 4) extends to FOUR stages by adding
// `stageRequestBody` + `stageResponseBody`. The per-direction
// at-most-once-per-stage discipline (parent §5.P10 + ADR-0171 §Decision (v))
// EXTENDS unchanged at 19.2 — the `f.overrideApplied [numStages]bool` array
// auto-resizes via the sentinel constant.
//
// Trailer stages remain RESERVED-BUT-OUT-OF-ENVELOPE per parent §5.P9 +
// ADR-0168 §Decision (trailer arms PARSE-REJECT permanently — trailers never
// reach the dispatch path); no enum entries land for trailers at 19.2.
type stage int

// stage enum values — at 19.2 IMPL (Task 4) extends from 2 → 4. The
// flat 4-value enum is the natural data model; the SPLIT-BY-DIRECTION
// discipline per planner-time D3 is enforced at the dispatch site (decode
// side tracks `stageRequestHeaders` + `stageRequestBody`; encode side tracks
// `stageResponseHeaders` + `stageResponseBody`) by the existing per-stage
// transition logic — the enum itself stays flat for index simplicity on the
// `f.overrideApplied` array + the `signalResume` switch.
const (
	// stageRequestHeaders fires on DecodeHeaders entry when
	// f.activeProcessingMode.RequestHeaderMode != SKIP.
	stageRequestHeaders stage = iota
	// stageResponseHeaders fires on EncodeHeaders entry when
	// f.activeProcessingMode.ResponseHeaderMode != SKIP.
	stageResponseHeaders
	// stageRequestBody fires on DecodeData endStream when
	// f.activeProcessingMode.RequestBodyMode == BUFFERED. Lands at 19.2
	// IMPL Task 4 per ADR-0171 §Decision AMENDMENT (body-mode portion).
	stageRequestBody
	// stageResponseBody fires on EncodeData endStream when
	// f.activeProcessingMode.ResponseBodyMode == BUFFERED. Lands at 19.2
	// IMPL Task 4 per ADR-0171 §Decision AMENDMENT (body-mode portion).
	stageResponseBody
	// Sentinel — bounds the `f.overrideApplied [numStages]bool` array. At
	// 19.2 IMPL Task 4 lifts from 2 → 4 (the 19.1 forward-pointer comment
	// "[numStages..numStages+4)" anticipated trailers + body together; the
	// actual Task 4 lift is 2 → 4 since trailers stay reserved-but-out-of-
	// envelope per parent §5.P9 + ADR-0168 §Decision — see ADR-0171
	// §Consequences refresh for the reconciliation).
	numStages
)

// String implements fmt.Stringer for grep-discoverable test failure output
// and the dispError wrapping in `dispatchStage` failure paths.
func (s stage) String() string {
	switch s {
	case stageRequestHeaders:
		return "request_headers"
	case stageResponseHeaders:
		return "response_headers"
	case stageRequestBody:
		return "request_body"
	case stageResponseBody:
		return "response_body"
	default:
		return fmt.Sprintf("stage(%d)", int(s))
	}
}

// ---------------------------------------------------------------------------
// action — applyProcessingResponse return classification per ADR-0171.
// ---------------------------------------------------------------------------

// action enumerates the post-applyProcessingResponse dispatch decision the
// completeStage goroutine consumes. Per SPEC §6.7 + ADR-0171 §Decision:
//
//   - actContinue: the stage's processing is complete (no further async work
//     pending); signal ContinueDecoding/ContinueEncoding on the resume
//     channel + increment streamsClosed if final.
//   - actStop: defer signal; the stage is parked waiting for an asynchronous
//     follow-up (e.g., a second ProcessingResponse). 19.1 does NOT exercise
//     this arm in the header-mode portion (reserved for 19.2 streaming body).
//   - actError: a protocol violation OR processor transport failure;
//     classified per cc.failureModeAllow at Task 8.
//   - actImmediate: an ImmediateResponse was emitted via SendLocalReply; the
//     stream is terminated; the dispatcher goroutine should NOT call
//     ContinueDecoding/ContinueEncoding (SendLocalReply takes over).
//   - actContinueButStillWaiting: a continue signal was emitted BUT the
//     stream is expected to receive further ProcessingResponses (rare;
//     reserved for the override_message_timeout short-circuit per parent
//     §5.P10 where the per-stage timer is reset and the SAME stage continues
//     waiting on the Recv loop for the substantive response).
type action int

// action enum values per ADR-0171 §Decision.
const (
	actContinue action = iota
	actStop
	actError
	actImmediate
	actContinueButStillWaiting
)

// String implements fmt.Stringer for test-failure log clarity.
func (a action) String() string {
	switch a {
	case actContinue:
		return "continue"
	case actStop:
		return "stop"
	case actError:
		return "error"
	case actImmediate:
		return "immediate"
	case actContinueButStillWaiting:
		return "continue-but-still-waiting"
	default:
		return fmt.Sprintf("action(%d)", int(a))
	}
}

// ---------------------------------------------------------------------------
// applyProcessingResponse — TEMPORARY STUB (Task 7). Replaced at Task 8.
// ---------------------------------------------------------------------------

// applyProcessingResponseFn is the per-stage ProcessingResponse dispatcher
// per SPEC §6.7. It is the SOLE entry point for translating a received
// *ProcessingResponse into an action + (on actError) a dispError sentinel.
//
// Declared as a function variable (NOT a plain function) so:
//
//   - Task 7 can install a TEMPORARY STUB that fails loudly with a
//     "stub at Task 7; lands at Task 8" sentinel — so any test that does
//     NOT explicitly override the stub gets a clear actError + sentinel-
//     wording error that pins the unimplemented contract.
//   - Group 7 tests can install per-test overrides (capturing the args +
//     returning deterministic action + error) to drive completeStage,
//     override_message_timeout, and mode_override coverage without dragging
//     in the Task 8 CommonResponse / header-mutation / ImmediateResponse
//     surface.
//   - Task 8 publishes the real body in `check.go` by REASSIGNING this
//     variable at package init (or by declaring its own non-stub function
//     and reassigning at init time). The variable indirection survives the
//     Task 8 takeover without altering call sites in processor.go.
//
// The signature matches the SPEC §6.7 contract: takes the *filter (for
// dcb/ecb access + activeProcessingMode mutation + the per-stage spurious
// counter) + the stage discriminator + the received *ProcessingResponse;
// returns the dispatch action + an optional error (non-nil only when action
// is actError).
var applyProcessingResponseFn = applyProcessingResponseStub

// applyProcessingResponseStub is the TEMPORARY STUB body installed at Task 7.
// Returns (actError, errProcessorStub) deterministically. Tests that drive
// completeStage with substantive coverage override `applyProcessingResponseFn`
// at the top of the test function (and restore on `t.Cleanup`).
//
// CLEANUP at Task 8: replaced by the real `applyProcessingResponse` in
// `check.go` which dispatches per SPEC §6.7 (ImmediateResponse +
// override_message_timeout + mode_override + CommonResponse + header
// mutation + clear_route_cache).
func applyProcessingResponseStub(f *filter, s stage, resp *extprocsvcv3.ProcessingResponse) (action, error) {
	_ = f
	_ = s
	_ = resp
	return actError, errProcessorStub
}

// errProcessorStub is the sentinel returned by `applyProcessingResponseStub`
// for unambiguous test assertions + a grep-discoverable failure surface in
// production logs (impossible to reach in production after Task 8 lands).
var errProcessorStub = errors.New("extproc: applyProcessingResponse stub at Task 7; real body lands at Task 8")

// ---------------------------------------------------------------------------
// resolveProcessingMode — parse + validate per parent §5.P9 + ADR-0171.
// ---------------------------------------------------------------------------

// Sentinel errors for resolveProcessingMode PARSE-REJECT paths. Tests assert
// on the prefix `"ext_proc: processing_mode: "` via errors.Is / strings.Contains.
//
// Post-19.2 §Decision AMENDMENT (ADR-0168): the body-mode gate flips from
// "must be NONE" (the 19.1 wording) to "must be NONE or BUFFERED" — the
// STREAMED-class arms (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED)
// continue PARSE-REJECT permanently per parent §4.4. The BUFFERED arm
// ACCEPTS for gRPC-service mode (the http_service arm continues
// PARSE-REJECT via errProcessingModeHTTPServiceBody — that gate now
// load-bearing post-lift; pre-lift it was subsumed by the listener-level
// body-mode-NONE check firing first).
var (
	errProcessingModeRequestBodyStreamedClass = errors.New(
		"ext_proc: processing_mode: request_body_mode must be NONE or BUFFERED (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED permanently out of envelope per parent §4.4 + ADR-0168 §Decision AMENDMENT)")
	errProcessingModeResponseBodyStreamedClass = errors.New(
		"ext_proc: processing_mode: response_body_mode must be NONE or BUFFERED (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED permanently out of envelope per parent §4.4 + ADR-0168 §Decision AMENDMENT)")
	errProcessingModeRequestTrailerNotSKIP = errors.New(
		"ext_proc: processing_mode: request_trailer_mode must be SKIP (trailers permanently out of envelope)")
	errProcessingModeResponseTrailerNotSKIP = errors.New(
		"ext_proc: processing_mode: response_trailer_mode must be SKIP (trailers permanently out of envelope)")
	errProcessingModeHTTPServiceBody = errors.New(
		"ext_proc: processing_mode: http_service mode requires request_body_mode=NONE + response_body_mode=NONE (per proto's ExtProcHttpService constraint; PERMANENT — the 19.2 §Decision AMENDMENT lift applies to the gRPC-service arm only)")
)

// resolveProcessingMode parses + validates an *extprocv3.ProcessingMode per
// parent §5.P9 + ADR-0171 §Decision header-mode portion + ADR-0168 §Decision
// AMENDMENT (phase-19.2 body-mode PARSE-REJECT lift). Translates per the
// proto-doc default semantics:
//
//   - HeaderSendMode.DEFAULT (0) → SEND for *_header_mode fields (request +
//     response).
//   - HeaderSendMode.DEFAULT (0) → SKIP for *_trailer_mode fields (request +
//     response).
//   - BodySendMode has no DEFAULT — zero value is NONE. Phase 19.1 PARSE-
//     REJECTed body-mode != NONE; phase 19.2 §Decision AMENDMENT lifts the
//     PARSE-REJECT for BUFFERED only (gRPC-service arm); the STREAMED-class
//     arms (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED) continue
//     PARSE-REJECT PERMANENTLY per parent §4.4.
//
// `httpServiceMode` (true when the listener's transport is http_service) adds
// the proto's ExtProcHttpService constraint: HTTP mode forces body-mode
// NONE (PARSE-REJECT otherwise). The httpServiceMode gate is LOAD-BEARING
// post-19.2 §Decision AMENDMENT — pre-AMENDMENT it was masked by the
// listener-level body-mode-NONE check firing first. The trailer-mode +
// DEFAULT translation disciplines are the SAME across both transport modes.
//
// A nil ProcessingMode input yields the all-defaults resolved value (matches
// the proto-doc behavior: a missing ProcessingMode field defaults to "send
// headers, skip trailers, no body").
//
// Returns nil + a sentinel error on PARSE-REJECT; the error wording starts
// with `"ext_proc: processing_mode: "` for grep-discoverability at the
// buildCompiledConfig call site.
func resolveProcessingMode(pm *extprocv3.ProcessingMode, httpServiceMode bool) (*resolvedProcessingMode, error) {
	// Capture raw enum values (allow nil pm — defaults to all-zero).
	var (
		rawReqHdr, rawRespHdr     extprocv3.ProcessingMode_HeaderSendMode
		rawReqBody, rawRespBody   extprocv3.ProcessingMode_BodySendMode
		rawReqTrail, rawRespTrail extprocv3.ProcessingMode_HeaderSendMode
	)
	if pm != nil {
		rawReqHdr = pm.GetRequestHeaderMode()
		rawRespHdr = pm.GetResponseHeaderMode()
		rawReqBody = pm.GetRequestBodyMode()
		rawRespBody = pm.GetResponseBodyMode()
		rawReqTrail = pm.GetRequestTrailerMode()
		rawRespTrail = pm.GetResponseTrailerMode()
	}

	// http_service + body-mode != NONE PARSE-REJECT per the proto's
	// ExtProcHttpService constraint (PERMANENT — the 19.2 §Decision
	// AMENDMENT body-mode lift applies to the gRPC-service arm only).
	// Fires BEFORE the gRPC-arm STREAMED-class check below so the more
	// specific PARSE-REJECT wording (the http_service constraint) wins.
	if httpServiceMode && (rawReqBody != extprocv3.ProcessingMode_NONE || rawRespBody != extprocv3.ProcessingMode_NONE) {
		return nil, errProcessingModeHTTPServiceBody
	}

	// Body-mode validation per parent §4.4 + ADR-0168 §Decision AMENDMENT
	// (phase-19.2 lift). NONE + BUFFERED ACCEPT for gRPC-service mode;
	// STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED PARSE-REJECT
	// PERMANENTLY (out of envelope per parent §4.4).
	if !bodyModeIsNoneOrBuffered(rawReqBody) {
		return nil, errProcessingModeRequestBodyStreamedClass
	}
	if !bodyModeIsNoneOrBuffered(rawRespBody) {
		return nil, errProcessingModeResponseBodyStreamedClass
	}

	// Trailer-mode validation per parent §5.P9 — DEFAULT translates to SKIP;
	// SKIP is honored; SEND PARSE-REJECT permanently (trailers out of
	// envelope per parent §5.P9 + ADR-0168 §Decision).
	if !trailerModeIsSkipOrDefault(rawReqTrail) {
		return nil, errProcessingModeRequestTrailerNotSKIP
	}
	if !trailerModeIsSkipOrDefault(rawRespTrail) {
		return nil, errProcessingModeResponseTrailerNotSKIP
	}

	return &resolvedProcessingMode{
		RequestHeaderMode:   headerModeTranslate(rawReqHdr),
		ResponseHeaderMode:  headerModeTranslate(rawRespHdr),
		RequestBodyMode:     rawReqBody,
		ResponseBodyMode:    rawRespBody,
		RequestTrailerMode:  extprocv3.ProcessingMode_SKIP,
		ResponseTrailerMode: extprocv3.ProcessingMode_SKIP,
	}, nil
}

// bodyModeIsNoneOrBuffered returns true iff the raw body-mode enum value is
// in the post-§Decision AMENDMENT ACCEPT set: NONE (the zero value; body-stage
// inactive) or BUFFERED (full-body inspection per ADR-0128 reuse on decode +
// ADR-0175 primitive on encode). STREAMED, BUFFERED_PARTIAL, and
// FULL_DUPLEX_STREAMED return false — these are PERMANENTLY out of envelope
// per parent §4.4 (STREAMED-class body modes; envoy-go-strict exclusion).
func bodyModeIsNoneOrBuffered(m extprocv3.ProcessingMode_BodySendMode) bool {
	return m == extprocv3.ProcessingMode_NONE || m == extprocv3.ProcessingMode_BUFFERED
}

// headerModeTranslate maps a HeaderSendMode to the resolved enum per parent
// §5.P9: DEFAULT → SEND for *_header_mode fields; SEND / SKIP pass through.
// Any unknown future enum value defensively maps to SEND (the proto-doc
// default for headers) to fail-safe-open for forward-compat — the
// PARSE-REJECT discipline for unknown enum values is the caller's choice
// (the SPEC §5.P9 contract does not require PARSE-REJECT here since the
// proto's enum scheme is closed at v3).
func headerModeTranslate(m extprocv3.ProcessingMode_HeaderSendMode) extprocv3.ProcessingMode_HeaderSendMode {
	switch m {
	case extprocv3.ProcessingMode_DEFAULT:
		return extprocv3.ProcessingMode_SEND
	case extprocv3.ProcessingMode_SKIP:
		return extprocv3.ProcessingMode_SKIP
	case extprocv3.ProcessingMode_SEND:
		return extprocv3.ProcessingMode_SEND
	default:
		return extprocv3.ProcessingMode_SEND
	}
}

// trailerModeIsSkipOrDefault returns true iff the raw trailer-mode enum
// value is DEFAULT (which translates to SKIP per parent §5.P9) or the
// explicit SKIP enum value. SEND (or any future enum value) is PARSE-REJECT.
func trailerModeIsSkipOrDefault(m extprocv3.ProcessingMode_HeaderSendMode) bool {
	return m == extprocv3.ProcessingMode_DEFAULT || m == extprocv3.ProcessingMode_SKIP
}

// ---------------------------------------------------------------------------
// openProcessorStream — bidi-stream lifecycle per SPEC §6.8 + ADR-0167.
// ---------------------------------------------------------------------------

// openProcessorStream opens the per-stream bidi-stream against the configured
// *grpcclient.ProcessorClient per SPEC §6.8 sketch. Derives streamCtx from
// parentCtx + a cancel hook (consumed by OnDestroy + dispatchStage timeout
// rebuilds per D6). The Send error (if any) is caught at first dispatchStage
// Send rather than here — matches the SPEC §6.8 sketch verbatim.
//
// NO-OP in http_service mode (cc.grpcClient == nil) — that arm uses per-
// stage HTTP POSTs constructed at Task 8 in `check.go`. Callers (Task 8 +
// Task 11) gate the invocation on the configured transport mode.
//
// Precondition: f.parentCtx non-nil (initialized at DecodeHeaders entry from
// the listener-level background context per ADR-0167); f.cc.grpcClient
// non-nil (only called in gRPC mode).
//
// Side effects:
//   - f.streamCtx + f.streamCancel populated (consumed by OnDestroy +
//     dispatchStage).
//   - f.stream populated (consumed by dispatchStage Send/Recv).
//   - f.activeMsgTimeout initialized to cc.messageTimeout if not already set.
//   - cc.stats.streamsStarted incremented (gated `if cc.stats != nil` per
//     ADR-0085 nil-tolerance).
//
// Returns the underlying *grpcclient.ProcessorClient.Process error (passed
// through verbatim). On error: f.stream remains nil; the caller (Task 8
// dispatcher) classifies per cc.failureModeAllow.
//
// **Per-route routing (Task 11 rework Carryforward G)**: the active client is
// f.activeProcessorClient (set at resolvePerRoute time) — per-route
// grpc_service override wins over listener-level cc.grpcClient. If
// activeProcessorClient is nil (resolvePerRoute did not run yet, OR
// pickActiveProcessorClient returned nil), fall back to f.cc.grpcClient so
// pre-resolve test paths + degenerate-cc cases stay nil-tolerant.
//
// **Carryforward L (Task 12)**: the fallback path is reachable ONLY in test
// paths in production — DecodeHeaders ALWAYS calls resolvePerRoute first which
// sets f.activeProcessorClient = pickActiveProcessorClient(cc, pr) (a non-nil
// pointer whenever cc.grpcClient is non-nil). A production fallback hit would
// signal a bug — a code path opened the bidi-stream WITHOUT first resolving
// the per-route choice, silently picking the listener-level client even when a
// per-route grpc_service override existed (the original Carryforward G bug
// shape). The warning log makes the silent fallthrough audible so any future
// production-side regression surfaces in operator logs without changing the
// nil-tolerance contract for the existing test paths.
func (f *filter) openProcessorStream() error {
	if f == nil || f.cc == nil {
		return errors.New("ext_proc: openProcessorStream: gRPC client not configured")
	}
	client := f.activeProcessorClient
	if client == nil {
		// Carryforward L: warn when the fallback fires — reachable only in
		// test paths in production per the discipline above.
		if f.cc.grpcClient != nil {
			log.Printf("ext_proc: openProcessorStream: f.activeProcessorClient unset; falling back to listener-level cc.grpcClient (resolvePerRoute did not run — should be unreachable in production per Carryforward L)")
		}
		client = f.cc.grpcClient
	}
	if client == nil {
		return errors.New("ext_proc: openProcessorStream: gRPC client not configured")
	}
	parent := f.parentCtx
	if parent == nil {
		parent = context.Background()
	}
	f.streamCtx, f.streamCancel = context.WithCancel(parent)
	stream, err := client.Process(f.streamCtx)
	if err != nil {
		// Roll back the cancel hook on error so OnDestroy does not panic on
		// a never-completed open. The streamCancel is invoked here to release
		// the derived context.
		f.streamCancel()
		f.streamCtx = nil
		f.streamCancel = nil
		if f.cc.stats != nil {
			f.cc.stats.streamsFailed.Inc()
		}
		return err
	}
	f.stream = stream
	// Initialize the active per-message timeout from the listener-level
	// cc.messageTimeout if it has not been set by a prior stage. Subsequent
	// override_message_timeout arrivals reset this via handleOverrideMessageTimeout
	// per D6 cancel-and-rebuild discipline.
	if f.activeMsgTimeout == 0 {
		f.activeMsgTimeout = f.cc.messageTimeout
	}
	if f.cc.stats != nil {
		f.cc.stats.streamsStarted.Inc()
	}
	return nil
}

// ---------------------------------------------------------------------------
// dispatchStage — async per-stage Send/Recv goroutine per SPEC §6.8.
// ---------------------------------------------------------------------------

// dispatchStage fires an async goroutine that performs Send + Recv on the
// open bidi-stream for the given stage's ProcessingRequest. Per SPEC §6.8
// sketch + ADR-0169 §Decision + D6 cancel-and-rebuild discipline:
//
//  1. Wrap the streamCtx with a per-message timeout (context.WithTimeout)
//     using f.activeMsgTimeout (which starts at cc.messageTimeout + is reset
//     by handleOverrideMessageTimeout on override arrivals).
//  2. f.stream.Send(req) — on err, classify as actError via completeStage.
//  3. cc.stats.streamMsgsSent++.
//  4. f.stream.Recv() — blocks until a ProcessingResponse arrives or the
//     timeout elapses or the stream is canceled.
//  5. On Recv err (timeout / cancel / transport): completeStage(stage, nil, err).
//  6. On Recv ok: cc.stats.streamMsgsReceived++; completeStage(stage, resp, nil).
//
// The dispatchStage goroutine is the SOLE owner of the Send/Recv pair per
// the single-in-flight-message-per-stage discipline (parent §5.P10 +
// ADR-0171 §Decision); the framework's sequential decode→encode dispatch
// invariant guarantees at most ONE dispatchStage goroutine is live at any
// time.
//
// Preconditions: f.stream non-nil + f.streamCtx non-nil (openProcessorStream
// succeeded). Tests inject a fake ProcessStream via direct field assignment.
//
// Returns immediately (the goroutine is fire-and-forget); the caller (decode/
// encode framework method) returns StopIteration to park on the resume
// channel; completeStage signals the resume per the SPEC §6.8 narrative.
func (f *filter) dispatchStage(s stage, req *extprocsvcv3.ProcessingRequest) {
	// HTTP-service mode dispatch path per ADR-0167 + SPEC §6.5: when
	// cc.httpClient is non-nil (set by buildCompiledConfig's http_service arm),
	// route the per-stage envelope through a per-call POST against the
	// processor's HTTP endpoint instead of the gRPC bidi-stream. The HTTP path
	// owns its OWN per-call context (NOT derived from f.streamCtx — HTTP-mode
	// does not maintain a long-lived stream context; each stage is a fresh POST).
	if f.cc != nil && f.cc.httpClient != nil {
		f.dispatchHTTPStage(s, req)
		return
	}

	go func() {
		// Per D6 cancel-and-rebuild: build a fresh per-message context for the
		// Send/Recv pair. The deferred cancel guarantees the per-message
		// timer is released regardless of return path (success / error /
		// override_message_timeout reset rebuild).
		//
		// **19.2 Task 4 ADR-0171 §Decision AMENDMENT behavioral lift** per
		// planner-time D4: the per-message timer is consumed BEHAVIORALLY
		// (NOT structural-only as at 19.1) via a select on msgCtx.Done in
		// the Recv leg. msgCancel is captured on f.activeMsgCancel so
		// handleOverrideMessageTimeout can fire it on override-accept (single
		// rolling timer per direction — at-most-one-in-flight per ADR-0171
		// §Decision (v) makes per-stage timers redundant). On goroutine exit
		// the deferred msgCancel + activeMsgCancel-clear pair ensure the
		// in-flight cancel hook does NOT leak past the per-stage lifetime.
		msgTimeout := f.activeMsgTimeout
		// Defensive: streamCtx may be nil on an HTTP-mode misroute or test
		// path that bypasses openProcessorStream. Fall back to the parent
		// context so we never panic on a nil parent (mirrors the
		// openProcessorStream parent-fallback discipline at line 386).
		parent := f.streamCtx
		if parent == nil {
			parent = f.parentCtx
		}
		if parent == nil {
			parent = context.Background()
		}
		var msgCtx context.Context
		var msgCancel context.CancelFunc
		if msgTimeout > 0 {
			msgCtx, msgCancel = context.WithTimeout(parent, msgTimeout)
		} else {
			// Zero timeout = no per-message bound — fall back to parent ctx
			// alone (still cancelable via streamCancel on OnDestroy).
			msgCtx, msgCancel = context.WithCancel(parent)
		}
		// Publish the per-message cancel on the filter so
		// handleOverrideMessageTimeout can fire it on override-accept per
		// planner-time D4. Cleared on goroutine exit.
		f.mu.Lock()
		f.activeMsgCancel = msgCancel
		f.mu.Unlock()
		defer func() {
			msgCancel()
			// handleOverrideMessageTimeout CLEARS activeMsgCancel itself on
			// fire (cascading stream-fatal per the per-message-timer
			// behavioral lift); by the time this defer runs we either own
			// the slot or it was already cleared — unconditional nil
			// assignment is race-safe + idempotent.
			f.mu.Lock()
			f.activeMsgCancel = nil
			f.mu.Unlock()
		}()

		// Send the request. The gRPC client stream's Send is non-blocking
		// per the SPEC §6.8 + ADR-0169 §Decision narrative — the per-message
		// timer fires from the Recv path, not the Send path. Any Send error
		// is a transport-level failure (already-closed stream / queue full
		// under HTTP/2 backpressure).
		if err := f.stream.Send(req); err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.streamMsgsSent.Inc()
		}

		// Recv blocks until a ProcessingResponse arrives, the per-message
		// timer fires (cascade-cancels the stream via watchdog below), OR
		// the stream is canceled (OnDestroy + streamCancel).
		//
		// **19.2 Task 4 behavioral lift** per planner-time D4 + ADR-0171
		// §Decision AMENDMENT clause (vi): the per-message timer
		// enforcement runs as a WATCHDOG goroutine that observes
		// msgCtx.Done and invokes streamCancel on per-message deadline
		// expiry — terminating the bidi-stream which unblocks the in-flight
		// Recv on THIS goroutine with the stream-context error (per the
		// gRPC ClientStream contract). The trade-off (per-message timer
		// fail-fast terminates the WHOLE stream rather than only the
		// per-stage Recv) is accepted because:
		//
		//   - At-most-one-in-flight per direction per ADR-0171 §Decision
		//     (v) — only ONE stage is parked on Recv at any time; the
		//     per-message timer expiry IS the per-stream failure surface.
		//
		//   - The bidi-stream is one-shot per HTTP transaction (one stream
		//     per filter lifetime); a per-message timeout signals the
		//     processor has stalled or is unreachable, which is a
		//     stream-level failure.
		//
		//   - The same Send/Recv goroutine constraint (parent §5.P10 +
		//     ADR-0171 §Decision (iii) single-in-flight-message correlation)
		//     mandates Send + Recv on the SAME goroutine — no per-message
		//     interruption via Recv-leg goroutine substitution.
		//
		// The watchdog goroutine returns promptly when EITHER msgCtx.Done
		// fires (per-message deadline OR override-driven cancel — see
		// handleOverrideMessageTimeout) OR doneCh closes (Recv completed
		// normally + the dispatch goroutine signals the watchdog to exit).
		// The watchdog NEVER blocks past the per-stage lifetime.
		doneCh := make(chan struct{})
		go func() {
			select {
			case <-msgCtx.Done():
				// Per-message timer expired (deadline) OR
				// override_message_timeout cascade-canceled the per-message
				// timer. Cascade-cancel the stream so the in-flight Recv
				// unblocks. On normal Recv completion the doneCh closes
				// FIRST and msgCtx.Done is observed only by the goroutine
				// exit path (no streamCancel fires).
				if f.streamCancel != nil {
					f.streamCancel()
				}
			case <-doneCh:
				// Recv completed normally — exit the watchdog without
				// touching streamCancel.
			}
		}()

		resp, err := f.stream.Recv()
		close(doneCh)
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.streamMsgsReceived.Inc()
		}
		f.completeStage(s, resp, nil)
	}()
}

// ---------------------------------------------------------------------------
// dispatchHTTPStage — async per-stage POST/Response dispatch for the
// http_service-mode transport per ADR-0167 + SPEC §6.5 + the §19.P8
// RATIFIED-PENDING-IMPL-TIME pin closure (the protojson wire-shape closes
// at Task 13 fixture-harness scrape).
// ---------------------------------------------------------------------------

// dispatchHTTPStage is the HTTP-mode analog of dispatchStage. Per-stage
// dispatch is a stateless POST/Response pair against the processor's HTTP
// endpoint:
//
//  1. Marshal the per-stage *ProcessingRequest envelope via the ADR-0170
//     protojson codec (`marshalProcessingRequest`).
//  2. Build an http.Request to the cc.httpClient.baseURL with the marshaled
//     bytes as the body + `Content-Type: application/json` header.
//  3. Issue cc.httpClient.client.Do(req) — the per-call timeout is the
//     *http.Client.Timeout set at buildHTTPProcessorClient time per ADR-0167
//     (proto's HttpService.http_uri.timeout).
//  4. On non-2xx response → classify as transport error (streamsFailed++).
//  5. On 2xx → read body + unmarshalProcessingResponse → completeStage.
//
// Fire-and-forget goroutine pattern mirrors dispatchStage. The
// f.activeMsgTimeout is NOT consulted at HTTP-mode (the per-call timeout
// lives entirely on the *http.Client). The cc.stats counters mirror gRPC
// mode (streamsStarted on first stage, streamMsgsSent on each POST,
// streamMsgsReceived on each successful response, streamsClosed on the
// last stage's terminating response).
func (f *filter) dispatchHTTPStage(s stage, req *extprocsvcv3.ProcessingRequest) {
	go func() {
		// Lazily increment streamsStarted on first stage per ADR-0167 §Decision
		// (HTTP-mode does NOT have an explicit stream-open point — first POST
		// stands in for it).
		if !f.httpStreamStarted {
			f.httpStreamStarted = true
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsStarted.Inc()
			}
		}

		// Marshal the per-stage envelope to protojson per ADR-0170.
		body, err := marshalProcessingRequest(req)
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}

		parent := f.parentCtx
		if parent == nil {
			parent = context.Background()
		}

		httpReq, err := http.NewRequestWithContext(parent, http.MethodPost, f.cc.httpClient.baseURL, bytes.NewReader(body))
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := f.cc.httpClient.client.Do(httpReq)
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.streamMsgsSent.Inc()
		}

		defer func() { _ = resp.Body.Close() }()
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		if resp.StatusCode/100 != 2 {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, fmt.Errorf("ext_proc http_service: non-2xx status %d", resp.StatusCode))
			return
		}

		out, err := unmarshalProcessingResponse(respBytes)
		if err != nil {
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.streamsFailed.Inc()
			}
			f.completeStage(s, nil, err)
			return
		}
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.streamMsgsReceived.Inc()
		}
		f.completeStage(s, out, nil)
	}()
}

// ---------------------------------------------------------------------------
// completeStage — synchronous applyProcessingResponse + resume-channel signal
// per ADR-0171 §Decision.
// ---------------------------------------------------------------------------

// completeStage is invoked from the dispatchStage goroutine after the Send/
// Recv pair completes (successfully or with error). Per SPEC §6.8 + ADR-0171
// §Decision:
//
//  1. Acquire f.mu under the D9 race-guard discipline; if f.done is set
//     (OnDestroy already fired), return immediately — the resume signal would
//     be a no-op (the framework chain has been destroyed; ContinueDecoding/
//     Encoding would race against the chain teardown). Drop the response.
//
//  2. On err != nil (transport/timeout/cancel): the action is actError
//     unless cc.failureModeAllow flips it to actContinue. Either way the
//     resume signal fires (ContinueDecoding/Encoding) so the parked stage
//     unparks; the dispError classification is recorded at Task 8 in
//     applyProcessingResponse's failure-mode dispatcher.
//
//  3. On err == nil + resp != nil: invoke applyProcessingResponseFn(f, stage,
//     resp). The returned action drives the resume-channel signal:
//
//     - actContinue: signal ContinueDecoding (decode stages) or
//     ContinueEncoding (encode stages).
//     - actStop: NO signal (the stage is parked waiting for further
//     ProcessingResponses — reserved for 19.2 streaming body).
//     - actError: signal Continue + the dispError is logged at Task 8.
//     - actImmediate: signal Continue per the SendLocalReply+ContinueDecoding
//     pattern documented at chain.go's timerSendLocalReplyFilter test +
//     phase-18.x ext_authz (extauthz.go:1110-1111) + phase-09 fault
//     (fault.go:321-324). `dcb.SendLocalReply` (called from emitImmediateResponse)
//     sets `chain.localReplyDone=true` and synchronously runs the encode chain
//     via `chain.beginLocalReply` — but it does NOT unblock the HCM dispatch
//     goroutine, which is still parked in `parkDecode`/`parkEncode` waiting
//     on the resume channel. The resume signal here wakes that goroutine so
//     it can observe `localReplyDone=true` at the top of its loop and return
//     `(false, nil)`; HCM dispatch then reads `chain.LocalReplyResponse()`
//     and emits the wire bytes (status + headers + body) — the local-reply
//     path completes end-to-end. WITHOUT this signal the parked dispatch
//     goroutine deadlocks until ctx cancellation, surfacing as status=0
//     connection reset on the downstream client (the Task 13 fixture-0022
//     scenario-2 root cause).
//     - actContinueButStillWaiting: signal Continue + the stage continues
//     consuming further ProcessingResponses on the same stream (reserved
//     for the override_message_timeout reset flow per parent §5.P10).
func (f *filter) completeStage(s stage, resp *extprocsvcv3.ProcessingResponse, recvErr error) {
	// D9 race-guard: drop the response if OnDestroy has already fired. The
	// per-stream mutex covers the f.done flip + the dcb/ecb access — the
	// dispatch goroutine acquires the mutex BEFORE touching dcb/ecb; OnDestroy
	// acquires the mutex AFTER setting done=true (see OnDestroy below).
	f.mu.Lock()
	if f.done {
		f.mu.Unlock()
		return
	}
	f.mu.Unlock()

	// Transport / timeout / cancel error → actError + failure-mode posture.
	if recvErr != nil {
		// Task 8 lands the cc.failureModeAllow translation; at Task 7 we
		// signal the resume so the parked stage unparks (the dispatch is
		// stuck on StopIteration; signaling unblocks the framework chain).
		// The error itself is recorded via cc.stats.streamsFailed (already
		// incremented by dispatchStage) + would be passed to a logging /
		// dispError-classifying surface at Task 8.
		f.signalResume(s)
		return
	}

	// Substantive ProcessingResponse → dispatch via applyProcessingResponseFn.
	// Task 7 installs a sentinel-error stub; Task 8 publishes the real body.
	// Tests override applyProcessingResponseFn to drive deterministic action
	// returns for completeStage assertions.
	act, _ := applyProcessingResponseFn(f, s, resp)

	// **19.2 Task 7 body-mutation delivery (encode side):** when the
	// just-completed stage is stageResponseBody, the applyProcessingResponse
	// arm may have mutated f.encodeBodyBuf via the body_mutation switch.
	// Register the (possibly-mutated) buffer with the chain via ADR-0131
	// OverwriteBody BEFORE signalResume — otherwise the chain unwinds + HCM
	// reads chain.EncodeBodyOverride() before the override is registered,
	// missing the mutation. Per ADR-0131 §Decision (vi): OverwriteBody is
	// safe to call from the dispatch goroutine prior to the resume signal
	// (the dispatch goroutine is the SOLE writer to the chain's
	// encodeBodyOverride at this moment; the parked HCM dispatch is
	// happens-before-ordered after the resume-channel send).
	//
	// The request_body analog is OMITTED — the decode side has no
	// OverwriteBody equivalent (the KNOWN LIMITATION documented at
	// (*filter).DecodeData). The decode-side body-mutation arm writes to
	// f.decodeBodyBuf which is visible to the per-stream filter but does
	// NOT reach the upstream; only the Content-Length header reconciliation
	// (via the f.decodeHeaders live map shared with HCM) is delivered.
	if s == stageResponseBody {
		f.deliverEncodeBodyMutation()
	}

	switch act {
	case actContinue, actError, actContinueButStillWaiting, actImmediate:
		// actImmediate REQUIRES the resume signal per the
		// SendLocalReply+ContinueDecoding pattern documented in chain.go's
		// timerSendLocalReplyFilter test (chain_test.go:849-876) and the
		// phase-18.x ext_authz precedent (extauthz.go:1110-1111 +
		// 1153-1154). The async dispatch goroutine calling SendLocalReply
		// from off-dispatch sets chain.localReplyDone=true and synchronously
		// runs the encode chain via beginLocalReply, but it does NOT unblock
		// the HCM dispatch goroutine which is still parked in parkDecode/
		// parkEncode. The resume signal here wakes that goroutine so it can
		// observe localReplyDone at the top of its loop and return; HCM
		// dispatch then reads chain.LocalReplyResponse() and emits the wire
		// bytes. WITHOUT this signal the parked dispatch goroutine deadlocks
		// until ctx cancellation, surfacing as status=0 connection reset on
		// the downstream client (the Task 13 fixture-0022 scenario-2 root
		// cause + the dropped scenario-2 byte-equivalence assertion).
		f.signalResume(s)
	case actStop:
		// No signal — stage parks waiting for further ProcessingResponses.
	}
}

// signalResume invokes the per-direction resume callback (ContinueDecoding
// for decode stages; ContinueEncoding for encode stages). Centralized so the
// completeStage switch above + any future direct-resume call sites share a
// single implementation. Nil-tolerant on dcb/ecb (test code paths may not
// have populated either).
//
// Per ADR-0171 §Decision AMENDMENT (phase-19.2 Task 4): the 4-stage extension
// keeps the per-direction discipline — decode stages (`stageRequestHeaders` +
// `stageRequestBody`) signal `ContinueDecoding`; encode stages
// (`stageResponseHeaders` + `stageResponseBody`) signal `ContinueEncoding`.
// Body stages reuse the SAME resume primitive as their header counterpart
// per the parent §5.P10 SPEC §6.8 invariant (one resume signal per parked
// stage's outbound).
func (f *filter) signalResume(s stage) {
	switch s {
	case stageRequestHeaders, stageRequestBody:
		if f.dcb != nil {
			f.dcb.ContinueDecoding()
		}
	case stageResponseHeaders, stageResponseBody:
		if f.ecb != nil {
			f.ecb.ContinueEncoding()
		}
	}
}

// ---------------------------------------------------------------------------
// handleOverrideMessageTimeout — at-most-ONCE-per-stage timer reset per
// parent §5.P10 + ADR-0171 §Decision.
// ---------------------------------------------------------------------------

// handleOverrideMessageTimeout consumes a ProcessingResponse.override_message_timeout
// per the parent §5.P10 RATIFIED discipline + ADR-0171 §Decision header-mode
// portion. Returns true iff the override was accepted (gating + per-stage
// idempotency tracking per parent §5.P10).
//
// Gating per parent §5.P10:
//
//   - cc.maxMessageTimeout < 1ms → override DISABLED entirely; increment
//     overrideMessageTimeoutIgnored++; return false.
//   - duration < 1ms OR duration > cc.maxMessageTimeout → out-of-range;
//     increment overrideMessageTimeoutIgnored++; return false.
//   - stage already had an override applied → at-most-ONCE-per-stage
//     violation; increment overrideMessageTimeoutIgnored++; return false.
//
// On accept (all gates pass):
//
//   - overrideMessageTimeoutReceived++.
//   - f.overrideApplied[stage] = true.
//   - f.activeMsgTimeout = duration (consumed by any FUTURE dispatchStage
//     invocation on a freshly-opened stream — see stream-fatal note below).
//   - f.activeMsgCancel fired (if non-nil) → cascades stream-fatal.
//
// **Stream-fatal cascade per Task 4 watchdog pattern (ADR-0171 §Decision
// AMENDMENT bullet 5).** Invoking the in-flight `activeMsgCancel` triggers
// `msgCtx.Done()` in the dispatchStage watchdog goroutine, which fires
// `f.streamCancel()` and cascade-terminates the WHOLE bidi-stream
// (the in-flight Recv unblocks with context.Canceled per the gRPC
// ClientStream contract). The "rebuild with override duration" semantic
// therefore applies ONLY to a future-stream lifecycle (if the stream is
// reopened by a subsequent request — a per-listener scope, not a
// continuation of the current in-flight stage): the
// override_message_timeout-driven cancel is effectively a stream-fatal
// classification. Per parent §5.P10 + ADR-0171 §Decision (v)
// at-most-one-in-flight + the bidi-stream's one-shot-per-HTTP-transaction
// lifetime (ADR-0167 §Decision), this is the intended behavior: an
// operator who configures override_message_timeout for tighter mid-stream
// bounds accepts the all-or-nothing stream-fatal classification on cancel.
// The alternative (per-stage Recv interruption without canceling the
// stream) would break the Send/Recv-same-goroutine invariant that 19.1's
// TestSequentialDecodeEncodeDispatchNoRace +
// TestBidiStreamSendRecvDiscipline assert.
//
// Per parent §5.P10 the override_message_timeout ProcessingResponse has its
// OTHER fields IGNORED; the caller (applyProcessingResponse) short-circuits
// the rest of the response if the override field is set + accepted.
//
// Nil-tolerant on nil receiver / nil cc / nil duration / nil stats (test
// paths exercise each).
func (f *filter) handleOverrideMessageTimeout(s stage, ot *durationpb.Duration) bool {
	if f == nil || f.cc == nil || ot == nil {
		return false
	}
	// Gate 1: max_message_timeout >= 1ms (otherwise override API disabled).
	if f.cc.maxMessageTimeout < time.Millisecond {
		if f.cc.stats != nil {
			f.cc.stats.overrideMessageTimeoutIgnored.Inc()
		}
		return false
	}
	// Gate 2: at-most-ONCE per stage.
	if s >= 0 && int(s) < len(f.overrideApplied) && f.overrideApplied[s] {
		if f.cc.stats != nil {
			f.cc.stats.overrideMessageTimeoutIgnored.Inc()
		}
		return false
	}
	// Gate 3: range check [1ms, max_message_timeout].
	d := ot.AsDuration()
	if d < time.Millisecond || d > f.cc.maxMessageTimeout {
		if f.cc.stats != nil {
			f.cc.stats.overrideMessageTimeoutIgnored.Inc()
		}
		return false
	}
	// Accept: mutate f.activeMsgTimeout (consumed by any FUTURE dispatchStage
	// on a freshly-opened stream — the in-flight cancel below cascades
	// stream-fatal, so there is NO continuation of the current stream
	// observing the new duration).
	if s >= 0 && int(s) < len(f.overrideApplied) {
		f.overrideApplied[s] = true
	}
	f.activeMsgTimeout = d
	if f.cc.stats != nil {
		f.cc.stats.overrideMessageTimeoutReceived.Inc()
	}
	// **19.2 Task 4 ADR-0171 §Decision AMENDMENT behavioral lift** per
	// planner-time D4: fire the in-flight per-message cancel (if captured by
	// the dispatchStage goroutine). The cancel triggers msgCtx.Done() in the
	// dispatchStage watchdog goroutine → watchdog fires f.streamCancel() →
	// whole bidi-stream terminates (in-flight Recv unblocks with
	// context.Canceled per the gRPC ClientStream contract). This is
	// STREAM-FATAL by design — see the docstring "Stream-fatal cascade" note
	// for the trade-off justification. CLEAR f.activeMsgCancel after fire so
	// the deferred clear in dispatchStage does not double-fire on a
	// non-owner slot.
	f.mu.Lock()
	cancel := f.activeMsgCancel
	f.activeMsgCancel = nil
	f.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}

// ---------------------------------------------------------------------------
// OnDestroy — sync.Once-guarded streamCancel + CloseSend per D9 discipline.
// ---------------------------------------------------------------------------

// onDestroyImpl is the substantive OnDestroy body per ADR-0171 §Decision +
// D9 race discipline. Invoked from the (*filter).OnDestroy framework hook
// in extproc.go (Task 2 left a noop stub there; this body lands at Task 7 +
// Task 12 race tests).
//
// Per D9 race discipline (NO per-stream mutex on the bidi-stream surface,
// only on the dcb/ecb signal surface):
//
//  1. sync.Once-guard the entire body via f.closeOnce — idempotent for
//     multiple OnDestroy invocations + concurrent OnDestroy + dispatch
//     completion races.
//
//  2. Inside the Once: acquire f.mu, set f.done = true, release f.mu. The
//     dispatch goroutine's completeStage above checks f.done under f.mu
//     BEFORE signaling resume, so a completion-vs-destroy race resolves
//     deterministically (the goroutine either sees done=true + drops the
//     signal, or signals before OnDestroy sets done — both outcomes are
//     valid per the framework's chain-destroyed-after-stream-ends contract).
//
//  3. Invoke f.streamCancel (if non-nil) to abort any in-flight Send/Recv.
//     The dispatch goroutine's Recv returns context.Canceled promptly; its
//     completeStage sees f.done = true (set above) and drops the signal.
//
//  4. Invoke f.stream.CloseSend (if non-nil) to send the half-close signal
//     to the processor. Best-effort: ignore the error (the stream may
//     already be closed via streamCancel). Per the gRPC ClientStream
//     contract, CloseSend is safe to call after the stream context is
//     canceled.
//
//  5. Increment cc.stats.streamsClosed if cc + stats are non-nil.
//
// The internal name `onDestroyImpl` exists so the public (*filter).OnDestroy
// in extproc.go can be the framework's required nullary hook + delegate
// here. Tests can also invoke onDestroyImpl directly for deterministic
// Group 10 lifecycle assertions.
func (f *filter) onDestroyImpl() {
	if f == nil {
		return
	}
	f.closeOnce.Do(func() {
		// D9: set f.done under f.mu so completeStage's pre-signal check
		// observes the flag. The mutex is brief: only the flag flip + (in
		// completeStage above) the flag read.
		f.mu.Lock()
		f.done = true
		f.mu.Unlock()

		// Abort any in-flight Send/Recv on the bidi-stream. Nil-tolerant
		// for the http_service mode path (which never opens a stream) +
		// the failed-open path (openProcessorStream rolled back).
		if f.streamCancel != nil {
			f.streamCancel()
		}

		// Send half-close to the processor — best-effort.
		if f.stream != nil {
			_ = f.stream.CloseSend()
		}

		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.streamsClosed.Inc()
		}
	})
}
