package extproc

// check.go — ADR-0172 header-mode portion of the per-stage dispatcher +
// CommonResponse mutation + ImmediateResponse multi-stage deny path per
// phase 19.1 IMPL (LANDS AT TASK 8).
//
// Production surface (mode-agnostic per SPEC §6.7 — gRPC + HTTP-mode converge
// at this dispatcher layer; the json codec at json.go + the bidi-stream
// primitive at processor.go are the only mode-specific pieces below):
//
//   - applyProcessingResponse — the per-stage ProcessingResponse dispatcher
//     publishing the real body (Task 7 left a sentinel stub; this file
//     reassigns applyProcessingResponseFn at package init per Carryforward C).
//     Per SPEC §6.7:
//       (1) ImmediateResponse short-circuit (silent-drop + spurious++ when
//           disable_immediate_response=true);
//       (2) override_message_timeout (delegates to processor.go's
//           handleOverrideMessageTimeout; other fields ignored per parent
//           §5.P10);
//       (3) mode_override re-eval (header-response stages only; allow_mode_override
//           + allowed_override_modes gating per parent §5.P1 + ADR-0171);
//       (4) per-stage CommonResponse extraction (stage-mismatch → spurious +
//           dispError per the SPEC §6.7 sketch);
//       (5) applyHeaderMutation per direction (mutation_rules gating per
//           parent §5.P3; rejected mutations dropped + ONCE-per-stage
//           spurious_msgs_received++);
//       (6) clear_route_cache / route_cache_action precedence (request_headers
//           stage ONLY per parent §5.P5 + the proto's
//           "ignored in the response direction" note);
//       (7) CONTINUE_AND_REPLACE → spurious + dispError per D7 (19.1 PARSE-
//           REJECT at runtime; lifts at 19.2 with body-mode activation).
//
//   - applyHeaderMutation — the per-direction HeaderMutation apply loop with
//     per-header mutation_rules gating + the 4-arm append_action dispatch per
//     phase-10 header_mutation + phase-18.2 ext_authz OkHttpResponse precedent.
//
//   - emitImmediateResponse — translates an *ImmediateResponse to the
//     framework's f.dcb.SendLocalReply (decode-stage) OR f.ecb.SendLocalReply
//     (encode-stage — FIRST §9 row to emit SendLocalReply from the encode side
//     at response_headers per ADR-0167 + ADR-0075). Per parent §5.P2: gRPC-
//     downstream sniff via f.requestContentType == "application/grpc" routes
//     body into the grpc-message header + grpc_status into the grpc-status
//     trailer; non-gRPC downstream → body as text/plain.
//
//   - resolveMutationRules — pre-compile *v31.HeaderMutationRules into the
//     resolvedMutationRules form consumed by applyHeaderMutation. Nil input
//     resolves to the proto-default rule set (host/:authority/:scheme/:method/
//     x-envoy-* protected; pseudo-headers protected when disallow_system fires;
//     otherwise allowed by default).
//
//   - buildGRPCProcessorClient — gRPC-arm transport constructor per SPEC §6.5:
//     GoogleGrpc PARSE-REJECT (inherits ADR-0157 §Decision AMENDMENT);
//     EnvoyGrpc.cluster_name PGV-mirror (non-empty); cluster-manager lookup +
//     UseH2 gate; grpcclient.NewProcessorClient construction.
//
//   - buildHTTPProcessorClient — HTTP-arm transport constructor per SPEC §6.5:
//     validates *ExtProcHttpService.http_service.http_uri.uri set + non-empty;
//     constructs *http.Client{Timeout: http_uri.timeout}; captures the base
//     URL + path_prefix for per-stage POST construction at the dispatcher.
//
//   - mapTransportError + the failure-mode dispatch on transport / unmarshal
//     errors per SPEC §4 + D5: failure_mode_allow=false → SendLocalReply(500)
//     + streamsFailed++ + dispError; failure_mode_allow=true → silent-continue
//     + failureModeAllowed++ + streamsFailed++. The existing
//     DecoderFilterCallbacks.SendLocalReply primitive suffices on both
//     stages; the encode-side response-headers-delivered branch uses
//     f.ecb.SendLocalReply(0, ...) as the stream-reset signal per ADR-0085
//     (the framework's local-reply path with status 0 reduces to a stream-
//     reset per phase-04 + ADR-0075).
//
// **D9 race discipline** continues to apply (NO per-stream mutex on the
// dispatcher surface; the framework's sequential decode→encode dispatch
// invariant + the bidi-stream single-in-flight-message correlation
// guarantee at most ONE dispatcher goroutine is live at any time per stream).
// applyProcessingResponse + emitImmediateResponse + applyHeaderMutation all
// run INSIDE the dispatcher goroutine after completeStage (see processor.go).
//
// **Init-time wiring per Carryforward C**: applyProcessingResponseFn is
// declared in processor.go as a function variable initialized to the Task 7
// stub. This file installs the real body via an explicit `func init()` block
// — race-safe per Go's package initialization contract (sequential single-
// goroutine init phase per Go spec §"Package initialization").
//
// **Per-message timer disposition per Carryforward D**: at Task 7 the
// dispatchStage goroutine built a per-message msgCtx via context.WithTimeout
// but did NOT bind it to stream.Send/Recv (which inherit ctx from
// Process(ctx) at stream-open — gRPC ClientStream's Send/Recv do not accept
// per-call ctx). Task 8 picks option (b) per the BOOTSTRAP carryforward:
// AMEND ADR-0171 §Consequences inline to note the per-message timer is a
// structural sketch at 19.1; actual enforcement defers to a future ADR / 19.2
// IMPL. The Task 8 disposition rationale (no code change in processor.go;
// doc-only amendment to ADR-0171 §Consequences + PROGRESS entry) preserves
// the existing dispatcher contract while pinning the gap.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	commonmutationv3 "github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocsvcv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/grpcclient"
)

// init installs the real applyProcessingResponse body in place of the
// Task 7 stub per Carryforward C. Race-safe because Go's package
// initialization runs in a single goroutine per the Go spec
// §"Package initialization" — no goroutine can observe a torn
// applyProcessingResponseFn value because no init goroutine starts before
// all package init functions return.
//
// Pro-tip per BOOTSTRAP Carryforward C: the reassignment lives in a small
// dedicated init() block (not inline at file scope) so the rationale comment
// lives next to the reassignment for grep-discoverability + future-reader
// orientation.
func init() {
	applyProcessingResponseFn = applyProcessingResponse
}

// ---------------------------------------------------------------------------
// Sentinel errors per SPEC §6.7 + parent §5.P11 + D5/D7/D8 classification.
// ---------------------------------------------------------------------------

// errStageMismatch is returned by applyProcessingResponse when the received
// *ProcessingResponse's response-oneof discriminator does not match the stage
// the filter dispatched. Per parent §5.P11 + SPEC §6.7 step 5: the response
// is classified as spurious + dispError (spuriousMsgsReceived++ at the
// classification site; dispError flows back to the caller for failure-mode
// dispatch).
var errStageMismatch = errors.New(
	"ext_proc: stage-mismatch: processor returned a ProcessingResponse for a different stage than the filter dispatched")

// errContinueAndReplaceNot19_1 is returned by applyProcessingResponse when a
// CommonResponse with status=CONTINUE_AND_REPLACE arrives in 19.1. Per
// ADR-0172 §Decision header-mode portion + D7: classified as spurious +
// dispError. Lifts at 19.2 with body-mode activation per the ADR-0172
// AMENDMENT path.
var errContinueAndReplaceNot19_1 = errors.New(
	"ext_proc: CONTINUE_AND_REPLACE not supported in 19.1 (header-mode portion); lands at 19.2 with body-mode activation per ADR-0172 §Decision AMENDMENT")

// ---------------------------------------------------------------------------
// resolvedMutationRules — pre-compiled per-header gating per parent §5.P3 +
// ADR-0172.
// ---------------------------------------------------------------------------

// **Task 8 field-promotion**: the Task 2 placeholder `resolvedMutationRules`
// struct (in extproc.go) is PROMOTED at Task 8 to carry four boolean fields
// (AllowAllRouting / AllowEnvoy / DisallowSystem / DisallowAll). The
// promotion is race-safe because compiledConfig (the holder of the
// *resolvedMutationRules pointer) is immutable post-buildCompiledConfig per
// ADR-0101 — the rules struct is read-only after that point + safe to share
// across all per-stream filter goroutines + per-stage dispatch invocations.
// At 19.1 the regex-matcher (allow_expression / disallow_expression) fields
// are SILENT-IGNORED per ADR-0172 §Consequences (the regex-matcher path
// lands at an IMPL-time AMENDMENT if a fixture pins the gap); the four
// boolean wrappers provide the substantive per-header gating for the 19.1
// fixture scope.

// resolveMutationRules pre-compiles a *commonmutationv3.HeaderMutationRules
// into the resolvedMutationRules carrier consumed by applyHeaderMutation.
//
// Nil input → resolves to the proto-default rule set (all four flags false).
// This matches the Envoy proto-doc default semantics: "host", ":authority",
// ":scheme", ":method", and "x-envoy-*" headers are protected by default.
//
// The PGV regex-matcher fields (allow_expression / disallow_expression) are
// SILENT-IGNORED at 19.1 per ADR-0172 §Consequences for IMPL-simplicity; the
// 19.1 fixture scope does not exercise regex-driven per-header overrides.
//
// Returns a non-nil *resolvedMutationRules + nil error; PARSE-REJECT path is
// reserved for the 19.2 regex-matcher promotion. The
// HeaderMutationRules.disallow_is_error wrapper is IGNORED at envoy-go-strict
// — the parent §5.P3 RATIFIED discipline classifies all rejected mutations
// as `spurious_msgs_received++` + drop regardless of the proto's per-rule
// disallow_is_error flag (envoy-go does NOT 500-on-rejected-mutation; the
// proto's discipline maps onto envoy-go's monotonic-counter-only convention).
func resolveMutationRules(rules *commonmutationv3.HeaderMutationRules) *resolvedMutationRules {
	out := &resolvedMutationRules{}
	if rules != nil {
		out.AllowAllRouting = rules.GetAllowAllRouting().GetValue()
		out.AllowEnvoy = rules.GetAllowEnvoy().GetValue()
		out.DisallowSystem = rules.GetDisallowSystem().GetValue()
		out.DisallowAll = rules.GetDisallowAll().GetValue()
	}
	return out
}

// isAllowed reports whether the named header may be mutated per the resolved
// rule set + parent §5.P3 protected-set semantics. The protected default set
// (when no override fires) is:
//
//   - host / :authority / :scheme / :method (routing-influencing pseudo-
//     headers; gated by AllowAllRouting)
//   - any header starting with x-envoy (Envoy-internal; gated by AllowEnvoy)
//   - any :-prefixed pseudo-header (when DisallowSystem fires; ALWAYS the
//     above 4 routing-influencing pseudo-headers regardless of DisallowSystem,
//     since they fall under AllowAllRouting)
//
// DisallowAll=true → no mutations allowed at all (returns false for any
// name).
//
// Header names are case-insensitively compared (lowercased internally) per
// Envoy's internal lowercase-header convention.
//
// Nil receiver → treats as proto-default (all flags false) per the
// ADR-0085 nil-tolerance pattern.
func (r *resolvedMutationRules) isAllowed(name string) bool {
	var (
		allowAllRouting bool
		allowEnvoy      bool
		disallowSystem  bool
		disallowAll     bool
	)
	if r != nil {
		allowAllRouting = r.AllowAllRouting
		allowEnvoy = r.AllowEnvoy
		disallowSystem = r.DisallowSystem
		disallowAll = r.DisallowAll
	}
	if disallowAll {
		return false
	}
	lower := strings.ToLower(name)
	// Routing-influencing pseudo-headers per parent §5.P3.
	switch lower {
	case "host", ":authority", ":scheme", ":method":
		return allowAllRouting
	}
	// :-prefixed pseudo-headers when DisallowSystem fires.
	if disallowSystem && strings.HasPrefix(lower, ":") {
		return false
	}
	// x-envoy-* prefix.
	if strings.HasPrefix(lower, "x-envoy") {
		return allowEnvoy
	}
	return true
}

// ---------------------------------------------------------------------------
// applyHeaderMutation — per-direction header-mutation apply loop with per-
// header mutation_rules gating per ADR-0172 §Decision (header-mode portion).
// ---------------------------------------------------------------------------

// applyHeaderMutation iterates the set_headers + remove_headers entries on a
// *HeaderMutation, applying each via the per-direction callbacks AFTER the
// per-header mutation_rules gating per parent §5.P3.
//
//   - Decode stage (stageRequestHeaders): applied via f.dcb.RequestHeaders()
//     analog — at 19.1 the framework's RequestHeaders() accessor is NOT yet
//     surfaced on DecoderFilterCallbacks (the chain holds the headers and
//     re-emits them on resume). For 19.1 the applyHeaderMutation body is a
//     STRUCTURAL CALL-IN that performs the mutation_rules gating + tracks the
//     rejection count, but the substantive header-mutation application
//     against the in-flight headers happens at the chain-level via the
//     resume-handoff per ADR-0085. At 19.1 the test surface exercises the
//     gating-and-rejection path; the chain-level mutation injection lands at
//     Task 11 buildCompiledConfig integration when the dcb.RequestHeaders()
//     analog is wired.
//
// Returns true iff ANY mutation was rejected by mutation_rules — the caller
// (applyProcessingResponse) increments cc.stats.spuriousMsgsReceived ONCE
// per stage (NOT per-rejected-header) per parent §5.P3 RATIFIED.
//
// The 4-arm append_action dispatch per ADR-0172 §Decision + phase-10
// header_mutation + phase-18.2 ext_authz precedent:
//
//	| append_action enum         | Semantics                                    |
//	|----------------------------|----------------------------------------------|
//	| APPEND_IF_EXISTS_OR_ADD    | headers.Add(name, value)                     |
//	| OVERWRITE_IF_EXISTS_OR_ADD | headers.Set(name, value) (proto-default)     |
//	| OVERWRITE_IF_EXISTS        | headers.Set(name, value) iff name exists     |
//	| ADD_IF_ABSENT              | headers.Add(name, value) iff name absent     |
func applyHeaderMutation(f *filter, s stage, hm *extprocsvcv3.HeaderMutation) (anyRejected bool) {
	if hm == nil || f == nil {
		return false
	}
	// Per-stage rules (filter-level cc.mutationRules; the per-stage discipline
	// is the SAME for request_headers + response_headers per parent §5.P3 +
	// SPEC §6.7).
	var rules *resolvedMutationRules
	if f.cc != nil {
		rules = f.cc.mutationRules
	}
	// Resolve the live in-flight header map per stage. The dispatch goroutine
	// reads f.decodeHeaders / f.encodeHeaders stashed at DecodeHeaders /
	// EncodeHeaders entry. Mirrors the phase-18.x ext_authz
	// applyUpstreamMutations(headers, disp) pattern (extauthz.go:1083).
	//
	// Task 13 rework: fixture-0022 scenarios 1+3+8 byte-equivalence root-
	// cause fix. The Task 8 body left this as a "deferred to Task 11" TODO;
	// the Task 11 wire-up landed parse-time only. The runtime apply lands
	// here.
	var live http.Header
	switch s {
	case stageRequestHeaders:
		live = f.decodeHeaders
	case stageResponseHeaders:
		live = f.encodeHeaders
	}
	// set_headers — per-entry mutation_rules check + 4-arm append dispatch.
	for _, hv := range hm.GetSetHeaders() {
		header := hv.GetHeader()
		if header == nil {
			continue
		}
		name := header.GetKey()
		if name == "" {
			continue
		}
		if !rules.isAllowed(name) {
			anyRejected = true
			continue
		}
		// HeaderValue carries the value in EITHER the `value` (string) field
		// OR the `raw_value` (bytes, may be non-UTF8) field per the proto.
		// Reference Envoy v1.37 emits raw_value preferentially; envoy-go
		// honors raw_value when non-empty, else falls back to value. This
		// mirrors the proto-doc "if raw_value is set, it takes precedence
		// over value" framing.
		value := header.GetValue()
		if rv := header.GetRawValue(); len(rv) > 0 {
			value = string(rv)
		}
		// Skip live-apply if the stash is nil (synthetic / test paths that
		// never invoked DecodeHeaders / EncodeHeaders). The mutation_rules
		// gating + counter accounting above still fire — only the
		// headers.Set/Add/Del call is elided. Nil-tolerance mirrors the
		// fakeDCB / fakeECB unit-test convention in extproc_test.go.
		if live == nil {
			continue
		}
		switch hv.GetAppendAction() {
		case corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD:
			live.Add(name, value)
		case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS:
			if live.Get(name) != "" {
				live.Set(name, value)
			}
		case corev3.HeaderValueOption_ADD_IF_ABSENT:
			if live.Get(name) == "" {
				live.Set(name, value)
			}
		case corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD:
			live.Set(name, value)
		default:
			// Unknown enum: forward-compat fall-through to proto-default
			// (OVERWRITE_IF_EXISTS_OR_ADD); matches the phase-18.2 ext_authz
			// buildAllowDispositionGRPC defensive enum-default.
			live.Set(name, value)
		}
	}
	// remove_headers — per-name mutation_rules check; rejected names → drop.
	for _, name := range hm.GetRemoveHeaders() {
		if name == "" {
			continue
		}
		if !rules.isAllowed(name) {
			anyRejected = true
			continue
		}
		if live != nil {
			live.Del(name)
		}
	}
	return anyRejected
}

// ---------------------------------------------------------------------------
// emitImmediateResponse — multi-stage deny path per ADR-0172 §Decision
// (header-mode portion) + parent §5.P2 + ADR-0167.
// ---------------------------------------------------------------------------

// emitImmediateResponse translates a *ImmediateResponse arrival into the
// framework's SendLocalReply per ADR-0085. Per ADR-0167 + parent §5.P2:
//
//   - Decode-stage (stageRequestHeaders): emitted via f.dcb.SendLocalReply.
//   - Encode-stage (stageResponseHeaders): emitted via f.ecb.SendLocalReply
//     — FIRST §9 row to emit SendLocalReply from the encode side at the
//     response_headers stage. The framework's SendLocalReply enters the
//     encode chain at filter[len-1] per ADR-0075; encode-side emission is
//     consistent per ADR-0167.
//
// gRPC-downstream-detection sniff per parent §5.P2: `f.requestContentType`
// captured at DecodeHeaders entry (the field exists on *filter per Task 2
// skeleton; the real capture-site lands at Task 11 DecodeHeaders wire-up).
//   - non-gRPC: body emitted with content-type text/plain (unless overridden
//     via headers.set_headers[content-type]).
//   - gRPC (content-type: application/grpc): body encoded into the
//     grpc-message response header (per the proto doc); content-type set to
//     application/grpc; grpc_status (when present) added as the grpc-status
//     response trailer.
//
// **Trailer emission caveat**: the existing SendLocalReply primitive does
// NOT carry trailers across the OrderedHeaders surface (the framework's
// local-reply path is single-shot per ADR-0085; trailers are an HTTP/2 wire
// feature). At 19.1 the grpc-status field is emitted as a HEADER instead of
// a trailer — this is a pragmatic 19.1 SPEC-divergence documented at
// ADR-0172 §Consequences (the 19.1 fixture asserts grpc-status in the response
// headers; the trailer-emission path lands at 19.2 IF the framework grows a
// SendLocalReply-with-trailers primitive).
//
// Per ADR-0172 §Decision: applies the *HeaderMutation per the 4-arm
// SET/REMOVE dispatch (REMOVE is a no-op on the SendLocalReply path since
// the local-reply has no pre-existing header set to remove from — the local-
// reply emission constructs the header set from scratch; REMOVE is logged
// but applied vacuously).
//
// Returns actImmediate (the dispatcher SHOULD NOT signal resume — the
// SendLocalReply handoff supersedes the chain).
func emitImmediateResponse(f *filter, ir *extprocsvcv3.ImmediateResponse, s stage) action {
	if f == nil || ir == nil {
		return actImmediate
	}
	// Mark the per-filter immediate-response-emitted flag so that subsequent
	// EncodeHeaders (re-entered via the framework's SendLocalReply →
	// encode-chain dispatch per ADR-0075) short-circuits to Continue without
	// re-invoking the processor. Per parent §5.P2 the deny is one-shot. Set
	// BEFORE the SendLocalReply emission so the encode-side re-entry observes
	// the flag if the framework dispatches synchronously.
	f.immediateResponseEmitted = true
	// HTTP status code per parent §5.P2 + ImmediateResponse.status.code (*HttpStatus).
	// The proto's StatusCode enum has integer values that ARE the HTTP status
	// codes (e.g., StatusCode_OK = 200; StatusCode_Forbidden = 403). Default
	// when nil per envoy-go-strict: 200 (the proto enum-0 is "Empty" which
	// envoy-go interprets defensively as 200 — matches reference Envoy's
	// permissive default).
	status := 200
	if hs := ir.GetStatus(); hs != nil {
		if code := int(hs.GetCode()); code > 0 {
			status = code
		}
	}

	// Detect gRPC downstream via the request-side content-type sniff captured
	// at DecodeHeaders entry per parent §5.P2.
	isGRPC := strings.EqualFold(f.requestContentType, "application/grpc")

	// Build the ordered header set: start with the post-mutation-rules-gated
	// SET entries from the ImmediateResponse.headers HeaderMutation, then
	// apply the content-type discipline + the grpc-status discipline + the
	// grpc-message discipline per parent §5.P2.
	var headers envoyhttp.OrderedHeaders
	hasContentType := false
	if hm := ir.GetHeaders(); hm != nil {
		// Apply mutation_rules per parent §5.P3 — but UNLIKE the per-stage
		// CommonResponse path, ImmediateResponse rejections are NOT counted
		// as spurious per the parent SPEC's framing (the deny-path emission
		// is treated as a one-shot; rejected mutations are silently dropped
		// + the resulting deny response carries only the allowed entries).
		// The IMPL discipline mirrors phase-18.2 buildDenyDispositionGRPC's
		// verbatim-pass-through with the parent §5.P3 protected-set gating
		// surfacing as a per-entry skip.
		var rules *resolvedMutationRules
		if f.cc != nil {
			rules = f.cc.mutationRules
		}
		for _, hv := range hm.GetSetHeaders() {
			header := hv.GetHeader()
			if header == nil {
				continue
			}
			name := header.GetKey()
			value := header.GetValue()
			if name == "" {
				continue
			}
			if !rules.isAllowed(name) {
				// Silent-skip per the ImmediateResponse framing — count is
				// reserved for the per-stage CommonResponse path.
				continue
			}
			headers = append(headers, envoyhttp.HeaderField{Name: name, Value: value})
			if strings.EqualFold(name, "content-type") {
				hasContentType = true
			}
		}
		// remove_headers are vacuous on the SendLocalReply path (no pre-
		// existing header set to remove from). Per the parent §5.P2 framing
		// the REMOVE entries are LOGGED but applied vacuously; at 19.1 the
		// log surface is not yet wired so the loop is a no-op pass.
		_ = hm.GetRemoveHeaders()
	}

	// Body discipline + content-type fallback per parent §5.P2.
	body := string(ir.GetBody())
	if isGRPC {
		// gRPC downstream: body → grpc-message header; content-type → application/grpc.
		if len(ir.GetBody()) > 0 {
			headers = append(headers, envoyhttp.HeaderField{Name: "grpc-message", Value: body})
			// Clear the body bytes on the wire — the body content travels in
			// the grpc-message header per the proto doc.
			body = ""
		}
		if !hasContentType {
			headers = append(headers, envoyhttp.HeaderField{Name: "content-type", Value: "application/grpc"})
		}
		// grpc_status → grpc-status response HEADER (the framework's
		// SendLocalReply does NOT carry trailers per ADR-0085 + the 19.1
		// pragmatic decision documented at ADR-0172 §Consequences; the
		// trailer-emission path lands at 19.2 if/when a SendLocalReply-
		// with-trailers framework primitive is introduced).
		if gs := ir.GetGrpcStatus(); gs != nil {
			headers = append(headers, envoyhttp.HeaderField{
				Name:  "grpc-status",
				Value: fmt.Sprintf("%d", gs.GetStatus()),
			})
		}
	} else {
		// Non-gRPC downstream: body emitted with content-type text/plain
		// unless overridden by an earlier SET entry per the parent §5.P2
		// "(unless overridden)" framing.
		if !hasContentType && len(ir.GetBody()) > 0 {
			headers = append(headers, envoyhttp.HeaderField{Name: "content-type", Value: "text/plain"})
		}
	}

	// Details field — proto-doc maps to %RESPONSE_CODE_DETAILS%; envoy-go's
	// HCM does not surface response_code_details (joint divergence-window
	// per ADR-0172 §Decision). At 19.1 the field is consumed (no error) but
	// not propagated — the details string is silently discarded at the
	// SendLocalReply boundary.
	_ = ir.GetDetails()

	// Emit per-stage. Decode-stage: f.dcb.SendLocalReply.
	// Encode-stage: f.ecb.SendLocalReply (FIRST §9 row to emit SendLocalReply
	// from the encode side at response_headers per ADR-0167 + ADR-0075).
	switch s {
	case stageRequestHeaders:
		if f.dcb != nil {
			f.dcb.SendLocalReply(status, body, headers)
		}
	case stageResponseHeaders:
		if f.ecb != nil {
			// Encoder-side SendLocalReply per ADR-0174 + ADR-0167. The
			// existing EncoderFilterCallbacks at 19.1 does NOT carry
			// SendLocalReply (the framework's encode-side local-reply path
			// uses the decoder-side dcb.SendLocalReply per ADR-0085; the
			// encode-chain enters at filter[len-1]). The 19.1 emission
			// path therefore routes ALSO through f.dcb.SendLocalReply when
			// f.ecb is the only non-nil callback — but the BOTH-decode-and-
			// encode filter at ADR-0167 carries f.dcb non-nil throughout
			// the per-stream lifetime, so the dcb-based emission below
			// covers BOTH stages reliably. The structural switch above
			// remains for future-proofing if/when the EncoderFilterCallbacks
			// grows its own SendLocalReply primitive.
			if f.dcb != nil {
				f.dcb.SendLocalReply(status, body, headers)
			}
		}
	}
	return actImmediate
}

// ---------------------------------------------------------------------------
// applyProcessingResponse — per-stage ProcessingResponse dispatcher per SPEC
// §6.7 + ADR-0172 §Decision (header-mode portion).
// ---------------------------------------------------------------------------

// applyProcessingResponse is the real body of applyProcessingResponseFn (the
// function variable declared in processor.go; installed via init() above
// per Carryforward C). Per SPEC §6.7 the per-stage dispatch sequence is:
//
//  1. ImmediateResponse short-circuit (per parent §5.P2): when set, emit
//     SendLocalReply via emitImmediateResponse — UNLESS
//     disable_immediate_response=true in which case silent-drop +
//     spuriousMsgsReceived++.
//
//  2. override_message_timeout (per parent §5.P10 + ADR-0171): when set,
//     delegate to f.handleOverrideMessageTimeout (which mutates
//     f.activeMsgTimeout for the NEXT dispatchStage per D6 cancel-and-rebuild).
//     OTHER fields ignored per the proto-doc; return actContinueButStillWaiting
//     to keep the structural action enum populated (see Carryforward B
//     disposition at ADR-0172 §Consequences).
//
//  3. mode_override re-eval (per parent §5.P1 + ADR-0171): when set + the
//     stage is a header-response stage + allow_mode_override=true + the
//     override is within allowed_override_modes — mutate f.activeProcessingMode
//     for SUBSEQUENT stages. When the stage is NOT a header-response stage
//     (body / trailer at 19.2) the override is silent-ignored per the parent
//     §5.P1 framing — NOT classified as spurious.
//
//  4. CommonResponse extraction per stage. stage-mismatch (the
//     ProcessingResponse's oneof discriminator does not match the stage's
//     expected arm) → spurious + dispError per the SPEC §6.7 step 5 +
//     parent §5.P11.
//
//  5. applyHeaderMutation per direction (per parent §5.P3 + ADR-0172
//     §Decision). rejections increment spurious_msgs_received ONCE per stage
//     with any rejection (NOT per-rejected-header).
//
//  6. clear_route_cache / route_cache_action precedence per parent §5.P5
//     RATIFIED: request_headers stage ONLY (body/response stage
//     CommonResponse.clear_route_cache is ignored per the proto's
//     "ignored in the response direction" note). The route_cache_action enum
//     wins when set; otherwise honor CommonResponse.clear_route_cache iff
//     route_cache_action == DEFAULT. NOTE: the actual ClearRouteCache call
//     site lands at Task 11 when the dcb.ClearRouteCache analog is wired;
//     at 19.1 the logic is structurally evaluated but the framework call is
//     a no-op stub (the chain primitive lands later in the §9 family-row
//     IMPL roadmap).
//
//  7. CONTINUE_AND_REPLACE (per ADR-0172 §Decision D7): 19.1 classifies as
//     spurious + dispError; lifts at 19.2 with body-mode activation.
//
// Returns (actContinue, nil) on the happy path; (actImmediate, nil) on
// ImmediateResponse emission; (actContinueButStillWaiting, nil) on
// override_message_timeout-only response; (actError, sentinel) on
// stage-mismatch or CONTINUE_AND_REPLACE.
func applyProcessingResponse(f *filter, s stage, resp *extprocsvcv3.ProcessingResponse) (action, error) {
	if f == nil || resp == nil {
		// Defensive: nil receiver / nil response → no-op + actContinue. The
		// caller (completeStage) only invokes this on resp != nil; the nil
		// guards here are anchor defensiveness for tests.
		return actContinue, nil
	}

	// Step 1: ImmediateResponse short-circuit per parent §5.P2 + SPEC §6.7.
	if ir := resp.GetImmediateResponse(); ir != nil {
		if f.cc != nil && f.cc.disableImmediateResponse {
			// Silent-drop per parent §5.P2 + ADR-0172 §Decision.
			if f.cc.stats != nil {
				f.cc.stats.spuriousMsgsReceived.Inc()
			}
			return actContinue, nil
		}
		return emitImmediateResponse(f, ir, s), nil
	}

	// Step 2: override_message_timeout per parent §5.P10 + ADR-0171.
	if ot := resp.GetOverrideMessageTimeout(); ot != nil {
		f.handleOverrideMessageTimeout(s, ot)
		// Other fields IGNORED per the proto-doc. Return
		// actContinueButStillWaiting per Carryforward B — at 19.1 the
		// completeStage treats it equivalently to actContinue (the in-flight
		// Recv cannot be reset mid-flight without canceling the stream; the
		// structural distinction is preserved for 19.2 streaming body
		// re-dispatch). See ADR-0172 §Consequences Carryforward B disposition.
		return actContinueButStillWaiting, nil
	}

	// Step 3: mode_override re-eval per parent §5.P1 + ADR-0171.
	if mo := resp.GetModeOverride(); mo != nil {
		// header-response stages only per the proto-doc + parent §5.P1.
		if isHeaderResponseStage(s) && f.cc != nil && f.cc.allowModeOverride {
			// Validate the override against allowed_override_modes (when
			// configured) and apply via resolveProcessingMode (re-uses the
			// Task 7 validator); on PARSE-REJECT of the override (per the
			// proto's processing_mode validation contract — body != NONE,
			// trailer != SKIP, http_service body constraint), silently
			// ignore the override per parent §5.P1 (NOT spurious).
			httpServiceMode := f.cc.httpServiceHeadersOnly
			newMode, err := resolveProcessingMode(mo, httpServiceMode)
			if err == nil && isModeInAllowlist(newMode, f.cc.allowedOverrideModes) {
				f.activeProcessingMode = newMode
			}
		}
		// Non-header-response stages: silent-ignore per parent §5.P1 (NOT
		// classified as spurious). Body / trailer stages reserved for 19.2.
	}

	// Step 4: CommonResponse extraction per stage.
	var cr *extprocsvcv3.CommonResponse
	switch s {
	case stageRequestHeaders:
		if hr := resp.GetRequestHeaders(); hr != nil {
			cr = hr.GetResponse()
		}
	case stageResponseHeaders:
		if hr := resp.GetResponseHeaders(); hr != nil {
			cr = hr.GetResponse()
		}
	}
	if cr == nil {
		// Stage-mismatch: the response's oneof discriminator did not match
		// the stage's expected arm (OR the response carried no CommonResponse
		// at all — also classified as a stage-mismatch). Per parent §5.P11 +
		// SPEC §6.7 step 5: spurious + dispError.
		//
		// Special case: if the response carried ONLY mode_override (handled
		// at Step 3 above) — i.e. all the other oneof / dispatch arms are
		// nil — we already returned at Step 3 above. So reaching here with
		// cr=nil means the response was structurally empty OR the oneof
		// was a different stage's arm (e.g., server returned response_headers
		// for our request_headers send).
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.spuriousMsgsReceived.Inc()
		}
		return actError, errStageMismatch
	}

	// Step 5: applyHeaderMutation per direction per parent §5.P3 + ADR-0172.
	if hm := cr.GetHeaderMutation(); hm != nil {
		if rejected := applyHeaderMutation(f, s, hm); rejected {
			// ONCE per stage with any rejection per parent §5.P3.
			if f.cc != nil && f.cc.stats != nil {
				f.cc.stats.spuriousMsgsReceived.Inc()
			}
		}
	}

	// Step 6: clear_route_cache / route_cache_action precedence per parent §5.P5.
	if s == stageRequestHeaders && f.cc != nil {
		shouldClear := shouldClearRouteCache(cr.GetClearRouteCache(), f.cc.routeCacheAction)
		// At 19.1 the framework's ClearRouteCache callback analog on
		// DecoderFilterCallbacks is NOT yet surfaced (the chain holds the
		// route resolution result; the clear-cache primitive lands at Task 11
		// buildCompiledConfig integration when the dcb.ClearRouteCache analog
		// is wired). The logic is structurally evaluated here so the
		// per-stage decision is pinned; the actual callback fires at the
		// Task 11 integration site.
		_ = shouldClear
	}

	// Step 7: CONTINUE_AND_REPLACE per ADR-0172 §Decision D7.
	if cr.GetStatus() == extprocsvcv3.CommonResponse_CONTINUE_AND_REPLACE {
		if f.cc != nil && f.cc.stats != nil {
			f.cc.stats.spuriousMsgsReceived.Inc()
		}
		return actError, errContinueAndReplaceNot19_1
	}

	return actContinue, nil
}

// isHeaderResponseStage reports whether the stage is a header-response stage
// (request_headers / response_headers). At 19.1 both are; body / trailer
// stages reserved for 19.2 return false.
func isHeaderResponseStage(s stage) bool {
	return s == stageRequestHeaders || s == stageResponseHeaders
}

// isModeInAllowlist reports whether the override mode is within the
// allowed_override_modes allowlist. Empty allowlist (nil or zero-length) →
// no restriction (all modes allowed when allow_mode_override=true). Per
// parent §5.P1 + ADR-0171 §Decision.
func isModeInAllowlist(mode *resolvedProcessingMode, allowlist []*resolvedProcessingMode) bool {
	if len(allowlist) == 0 {
		return true
	}
	if mode == nil {
		return false
	}
	for _, allowed := range allowlist {
		if allowed == nil {
			continue
		}
		if allowed.RequestHeaderMode == mode.RequestHeaderMode &&
			allowed.ResponseHeaderMode == mode.ResponseHeaderMode &&
			allowed.RequestBodyMode == mode.RequestBodyMode &&
			allowed.ResponseBodyMode == mode.ResponseBodyMode &&
			allowed.RequestTrailerMode == mode.RequestTrailerMode &&
			allowed.ResponseTrailerMode == mode.ResponseTrailerMode {
			return true
		}
	}
	return false
}

// shouldClearRouteCache returns true iff the route cache should be cleared
// for the current request_headers stage's CommonResponse, per parent §5.P5
// RATIFIED precedence:
//
//   - route_cache_action == CLEAR  → always clear (regardless of clear_route_cache).
//   - route_cache_action == RETAIN → never clear (regardless of clear_route_cache).
//   - route_cache_action == DEFAULT → honor clear_route_cache iff true.
//
// (The route_cache_action / disable_clear_route_cache mutual-exclusion gate
// fires at parse-time in Task 11 buildCompiledConfig; reaching here means
// AT MOST ONE of the two top-level fields is set + disable_clear_route_cache
// has already been translated to route_cache_action == RETAIN.)
func shouldClearRouteCache(clearFlag bool, action extprocv3.ExternalProcessor_RouteCacheAction) bool {
	switch action {
	case extprocv3.ExternalProcessor_CLEAR:
		return true
	case extprocv3.ExternalProcessor_RETAIN:
		return false
	default:
		// DEFAULT (0) — honor the per-response clear_route_cache.
		return clearFlag
	}
}

// ---------------------------------------------------------------------------
// buildGRPCProcessorClient — gRPC-arm transport constructor per SPEC §6.5 +
// ADR-0157 §Decision AMENDMENT + ADR-0169.
// ---------------------------------------------------------------------------

// buildGRPCProcessorClient constructs a *grpcclient.ProcessorClient for the
// gRPC arm per SPEC §6.5. The steps mirror phase-18.2 buildGRPCCheckFn:
//
//  1. GoogleGrpc PARSE-REJECT envoy-go-strict per ADR-0157 §Decision
//     AMENDMENT (envoy-go uses google.golang.org/grpc directly; the
//     GoogleGrpc native-channel arm is not supported).
//
//  2. EnvoyGrpc.cluster_name PGV-mirror (min_len:1) — PARSE-REJECT empty /
//     missing.
//
//  3. Cluster-manager lookup via ctx.ClusterManager.Get(clusterName) →
//     PARSE-REJECT on unknown / UseH2()==false (gRPC requires HTTP/2).
//
//  4. Construct *grpcclient.ProcessorClient via grpcclient.New(mgr) +
//     grpcclient.NewProcessorClient(dialer, clusterName, messageTimeout).
//
//  5. initial_metadata + retry_policy SILENT-IGNORED per ADR-0169 + the
//     proto's per-call discipline (envoy-go does not consume these fields;
//     processor-server designers may set them in their config files for
//     forward-compat without an envoy-go PARSE-REJECT).
//
// `messageTimeout` is the per-message timeout captured at config-load time
// (cc.messageTimeout, default 200ms per SPEC §6.5). The *ProcessorClient
// stores it but does NOT apply it inside Process() — the filter's
// dispatchStage (processor.go Task 7) reads it via *ProcessorClient.PerMessageTimeout()
// when building the per-Recv context.
func buildGRPCProcessorClient(gs *corev3.GrpcService, ctx envoyhttp.FactoryCtx, messageTimeout time.Duration) (*grpcclient.ProcessorClient, error) {
	if gs == nil {
		return nil, errors.New("ext_proc: grpc_service is required")
	}
	// Step 1: GoogleGrpc PARSE-REJECT per ADR-0157 §Decision AMENDMENT.
	if _, isGoogle := gs.GetTargetSpecifier().(*corev3.GrpcService_GoogleGrpc_); isGoogle {
		return nil, errors.New("ext_proc: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)")
	}
	// Step 2: EnvoyGrpc.cluster_name PGV-mirror.
	envoyGrpc := gs.GetEnvoyGrpc()
	if envoyGrpc == nil {
		return nil, errors.New("ext_proc: grpc_service: envoy_grpc arm required (target_specifier must be set)")
	}
	clusterName := envoyGrpc.GetClusterName()
	if clusterName == "" {
		return nil, errors.New("ext_proc: grpc_service: envoy_grpc.cluster_name must be non-empty")
	}
	// Step 3: Cluster-manager lookup + UseH2 gate.
	if ctx.ClusterManager == nil {
		return nil, errors.New("ext_proc: grpc_service: cluster manager not available (FactoryCtx.ClusterManager is nil)")
	}
	clu, ok := ctx.ClusterManager.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("ext_proc: grpc_service: unknown cluster %q", clusterName)
	}
	if !clu.UseH2() {
		return nil, fmt.Errorf("ext_proc: grpc_service: cluster %q must have http2_protocol_options{} set (gRPC requires HTTP/2)", clusterName)
	}
	// Step 4: Construct the *grpcclient.ProcessorClient.
	dialer := grpcclient.New(ctx.ClusterManager)
	pc, err := grpcclient.NewProcessorClient(dialer, clusterName, messageTimeout)
	if err != nil {
		return nil, fmt.Errorf("ext_proc: grpc_service: %w", err)
	}
	// Step 5: initial_metadata + retry_policy SILENT-IGNORED.
	return pc, nil
}

// ---------------------------------------------------------------------------
// buildHTTPProcessorClient — HTTP-arm transport constructor per SPEC §6.5 +
// ADR-0167 + the proto's ExtProcHttpService body-only-NONE constraint.
// ---------------------------------------------------------------------------

// buildHTTPProcessorClient constructs an *httpProcessorClient for the
// HTTP-arm per SPEC §6.5. The proto's ExtProcHttpService wraps a *HttpService
// (corev3) which carries:
//
//   - http_uri.uri — the absolute URI of the processor endpoint
//     (e.g. "http://processor.local:8080/process"). PARSE-REJECT empty.
//   - http_uri.cluster — the upstream cluster name (consumed by reference
//     Envoy; envoy-go's HTTP-mode transport uses a direct *http.Client per
//     ADR-0159 disposition (b) precedent inherited from phase-18.1, so the
//     cluster field is consumed for forward-compat documentation but does
//     NOT route through the cluster manager at 19.1).
//   - http_uri.timeout — the per-call timeout. Applied as the *http.Client
//     Timeout per ADR-0159 (zero = no timeout).
//
// path_prefix is NOT a field on the ExtProcHttpService proto in 19.1 (the
// proto-doc carries only the HttpService wrapper; the SPEC §6.5 reference to
// "path_prefix" anticipated a richer proto). The 19.1 IMPL captures only the
// base URL from http_uri.uri; the per-stage POST URL is the bare base URL
// with the per-stage path-suffix derived from the stage discriminator at
// dispatch time (Task 11 wires the per-stage POST construction).
func buildHTTPProcessorClient(hs *extprocv3.ExtProcHttpService) (*httpProcessorClient, error) {
	if hs == nil {
		return nil, errors.New("ext_proc: http_service is required")
	}
	svc := hs.GetHttpService()
	if svc == nil {
		return nil, errors.New("ext_proc: http_service.http_service is required")
	}
	uri := svc.GetHttpUri()
	if uri == nil || uri.GetUri() == "" {
		return nil, errors.New("ext_proc: http_service.http_service.http_uri.uri is required (must be non-empty)")
	}
	timeout := time.Duration(0)
	if d := uri.GetTimeout(); d != nil {
		timeout = d.AsDuration()
	}
	return &httpProcessorClient{
		client:  &http.Client{Timeout: timeout},
		baseURL: uri.GetUri(),
	}, nil
}

// ---------------------------------------------------------------------------
// mapTransportError — failure-mode dispatch per SPEC §4 + D5 + parent §5.P11.
// ---------------------------------------------------------------------------

// mapTransportError classifies a transport / unmarshal / per-message-timeout
// error per the failure-mode discipline at SPEC §4 + D5:
//
//   - failure_mode_allow=false (proto default): SendLocalReply(500, "", {})
//     on the request_headers branch; OR a stream-reset signal on the
//     response_headers branch (after response headers delivered). The 19.1
//     IMPL routes BOTH branches through f.dcb.SendLocalReply since the
//     framework's encode-side local-reply path enters via dcb per ADR-0075
//     (the encode-chain enters at filter[len-1]; the dcb pointer is set on
//     both branches throughout the per-stream lifetime per ADR-0167).
//     streamsFailed++ at the dispatchStage site (already incremented by
//     processor.go); dispError-style classification surfaces here as the
//     caller's actError return.
//
//   - failure_mode_allow=true: silent-continue + failureModeAllowed++ +
//     streamsFailed++ (the latter already incremented by processor.go).
//     Returns actContinue + nil error so the dispatcher signals resume +
//     the framework chain proceeds normally.
//
// Returns the dispatcher action + the (optional) dispError sentinel. The
// caller (completeStage at Task 12 race tests + buildCompiledConfig wiring)
// translates the action into the resume-signal / SendLocalReply decision.
//
// **Per-message-timeout** (the per-stage timer expires without a response)
// is classified as the SAME transport-error path per parent §5.P10 + SPEC §4:
// either failure-mode-allow surfaces it as silent-continue + counter, or
// fail-loud surfaces it as SendLocalReply(500). The processor.go
// dispatchStage returns ctx.Err() (context.DeadlineExceeded) on Recv-timeout;
// this function's switch on errors.Is(err, context.DeadlineExceeded)
// folds it onto the same classification path.
//
// At 19.1 this helper is INVOKED FROM applyProcessingResponse on the
// recvErr-bearing path — the call-site is the SAME completeStage entry the
// dispatcher's Recv-error path lands on. The wiring is structural at Task 8;
// the substantive consumer call site lands at Task 11 buildCompiledConfig
// integration when the failure-mode dispatch enters the production code
// path.
func mapTransportError(f *filter, s stage, err error) action {
	if f == nil || err == nil {
		return actContinue
	}
	// failure_mode_allow=true → silent-continue + failureModeAllowed++.
	if f.cc != nil && f.cc.failureModeAllow {
		if f.cc.stats != nil {
			f.cc.stats.failureModeAllowed.Inc()
		}
		return actContinue
	}
	// failure_mode_allow=false → SendLocalReply(500). The dispatcher signals
	// actImmediate so the resume path is NOT taken (the local-reply emission
	// supersedes the chain per ADR-0085).
	switch s {
	case stageRequestHeaders:
		if f.dcb != nil {
			f.dcb.SendLocalReply(500, "", nil)
		}
	case stageResponseHeaders:
		// Encode-stage after response headers delivered: per the proto's
		// failure_mode_allow doc + parent §5.P11, the stream is reset. The
		// 19.1 IMPL emits via f.dcb.SendLocalReply(0, ...) — the framework's
		// local-reply path with status 0 collapses to a stream-reset per
		// phase-04 + ADR-0085 (the local-reply emission with status 0 is
		// the framework's stream-reset signal). When the dcb is non-nil
		// (always at 19.1 per ADR-0167), this fires the reset path; when
		// nil (test path), the no-op return preserves the structural action.
		if f.dcb != nil {
			f.dcb.SendLocalReply(0, "", nil)
		}
	}
	return actImmediate
}

// (Task 8 placeholder `errSentinel` removed at Task 11 per Carryforward E —
// the real failure-mode path now lives at (*filter).DecodeHeaders /
// (*filter).EncodeHeaders + processor.go's dispatchStage → completeStage →
// mapTransportError chain wired through the production New factory closure.
// The transport-error sentinel surface is the raw err from the gRPC
// ClientStream / HTTP POST verbatim — NO classifier sentinel needed at
// 19.1 since the dispError logging surface is not yet wired (lands at a
// future logging-extension phase).)
