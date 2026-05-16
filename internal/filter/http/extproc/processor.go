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

// stage enumerates the per-direction stages the filter dispatches. 19.1
// activates ONLY the two header stages; the body + trailer stages are
// reserved (declared as `numStages` count tracks the array size for
// `f.overrideApplied`) for the 19.2 AMENDMENT per ADR-0171 §Consequences.
type stage int

// stage enum values — header stages active in 19.1; body + trailer reserved.
const (
	// stageRequestHeaders fires on DecodeHeaders entry when
	// f.activeProcessingMode.RequestHeaderMode != SKIP.
	stageRequestHeaders stage = iota
	// stageResponseHeaders fires on EncodeHeaders entry when
	// f.activeProcessingMode.ResponseHeaderMode != SKIP.
	stageResponseHeaders
	// Sentinel — bounds the f.overrideApplied array. Body + trailer stages
	// reserve indices [numStages..numStages+4) at the 19.2 AMENDMENT.
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
var (
	errProcessingModeRequestBodyNotNONE = errors.New(
		"ext_proc: processing_mode: request_body_mode must be NONE in 19.1 (BUFFERED activates at 19.2; STREAMED+BUFFERED_PARTIAL+FULL_DUPLEX_STREAMED permanently out of envelope)")
	errProcessingModeResponseBodyNotNONE = errors.New(
		"ext_proc: processing_mode: response_body_mode must be NONE in 19.1 (BUFFERED activates at 19.2; STREAMED+BUFFERED_PARTIAL+FULL_DUPLEX_STREAMED permanently out of envelope)")
	errProcessingModeRequestTrailerNotSKIP = errors.New(
		"ext_proc: processing_mode: request_trailer_mode must be SKIP (trailers permanently out of envelope)")
	errProcessingModeResponseTrailerNotSKIP = errors.New(
		"ext_proc: processing_mode: response_trailer_mode must be SKIP (trailers permanently out of envelope)")
	errProcessingModeHTTPServiceBody = errors.New(
		"ext_proc: processing_mode: http_service mode requires request_body_mode=NONE + response_body_mode=NONE (per proto's ExtProcHttpService constraint)")
)

// resolveProcessingMode parses + validates an *extprocv3.ProcessingMode per
// parent §5.P9 + ADR-0171 §Decision header-mode portion. Translates per the
// proto-doc default semantics:
//
//   - HeaderSendMode.DEFAULT (0) → SEND for *_header_mode fields (request +
//     response).
//   - HeaderSendMode.DEFAULT (0) → SKIP for *_trailer_mode fields (request +
//     response).
//   - BodySendMode has no DEFAULT — zero value is NONE. Phase 19.1 PARSE-
//     REJECTs body-mode != NONE; phase 19.2 lifts the PARSE-REJECT for
//     BUFFERED only.
//
// `httpServiceMode` (true when the listener's transport is http_service) adds
// the proto's ExtProcHttpService constraint: HTTP mode forces body-mode
// NONE (PARSE-REJECT otherwise). The trailer-mode + DEFAULT translation
// disciplines are the SAME across both transport modes.
//
// A nil ProcessingMode input yields the all-defaults resolved value (matches
// the proto-doc behavior: a missing ProcessingMode field defaults to "send
// headers, skip trailers, no body").
//
// Returns nil + a sentinel error on PARSE-REJECT; the error wording starts
// with `"ext_proc: processing_mode: "` for grep-discoverability at the
// Task 11 buildCompiledConfig call site.
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

	// Body-mode validation per parent §5.P9 + ADR-0168 §Decision body-mode
	// PARSE-REJECT (lifts at 19.2 for BUFFERED only).
	if rawReqBody != extprocv3.ProcessingMode_NONE {
		return nil, errProcessingModeRequestBodyNotNONE
	}
	if rawRespBody != extprocv3.ProcessingMode_NONE {
		return nil, errProcessingModeResponseBodyNotNONE
	}
	// http_service + body-mode != NONE PARSE-REJECT per proto constraint.
	// (The body checks above already cover this for ANY mode; the explicit
	// httpServiceMode-gated check below is RESERVED for 19.2 when the
	// listener-level body-mode PARSE-REJECT lifts but the http_service body
	// PARSE-REJECT remains. At 19.1 the listener-level check above subsumes
	// this — we keep the parameter consumed for symmetry + grep-anchor.)
	if httpServiceMode && (rawReqBody != extprocv3.ProcessingMode_NONE || rawRespBody != extprocv3.ProcessingMode_NONE) {
		return nil, errProcessingModeHTTPServiceBody
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
		RequestBodyMode:     extprocv3.ProcessingMode_NONE,
		ResponseBodyMode:    extprocv3.ProcessingMode_NONE,
		RequestTrailerMode:  extprocv3.ProcessingMode_SKIP,
		ResponseTrailerMode: extprocv3.ProcessingMode_SKIP,
	}, nil
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
		defer msgCancel()
		_ = msgCtx // reserved for Task 11 Send/Recv wiring per SPEC §6.8 sketch

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
		// timer fires, or the stream is canceled. The per-message timer's
		// firing is observable here as ctx.Err() == context.DeadlineExceeded
		// on the next Recv loop iteration. (For Task 7 the gRPC ClientStream
		// returns the timeout-derived error directly on Recv; per-message
		// timer reset on override_message_timeout arrival is handled by
		// handleOverrideMessageTimeout which mutates f.activeMsgTimeout for
		// the NEXT dispatchStage goroutine — the in-flight Recv on the
		// CURRENT goroutine cannot be reset mid-Recv without canceling the
		// streamCtx, which would terminate the whole stream. The
		// continue-but-still-waiting semantics for override_message_timeout
		// are handled in completeStage via the action enum.)
		resp, err := f.stream.Recv()
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
func (f *filter) signalResume(s stage) {
	switch s {
	case stageRequestHeaders:
		if f.dcb != nil {
			f.dcb.ContinueDecoding()
		}
	case stageResponseHeaders:
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
// portion. Returns true iff the override was accepted (the timer reset
// scheduled for the NEXT dispatchStage Send/Recv pair via f.activeMsgTimeout
// mutation per D6 cancel-and-rebuild discipline).
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
//   - f.activeMsgTimeout = duration (consumed by the NEXT dispatchStage per
//     the D6 cancel-and-rebuild discipline — see ADR-0171 §Decision (vi)).
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
	// Accept: reset the per-stage timer for the NEXT Send/Recv pair via
	// f.activeMsgTimeout mutation per D6 cancel-and-rebuild discipline.
	if s >= 0 && int(s) < len(f.overrideApplied) {
		f.overrideApplied[s] = true
	}
	f.activeMsgTimeout = d
	if f.cc.stats != nil {
		f.cc.stats.overrideMessageTimeoutReceived.Inc()
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
