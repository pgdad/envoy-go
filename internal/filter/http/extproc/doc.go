// Package extproc implements envoy.filters.http.ext_proc — Envoy v1.37.2's
// canonical "external processor" filter delegating per-stage processing
// (request_headers / request_body / response_headers / response_body) to an
// external bidi-streaming gRPC processor OR a per-stage HTTP POST processor —
// under the 07.1 HTTP filter framework. Phase 19.1. The filter is the TWELFTH
// §9 production HTTP filter (after cors / fault / header_mutation /
// local_ratelimit / csrf / buffer / compressor / bandwidth_limit / rbac /
// jwt_authn / ext_authz) and the FIRST §9 row to ship BOTH-DECODE-AND-ENCODE
// participation in a single filter package (phase-14 compressor's encode-only
// shape `Decoder: nil` is the only prior precedent for encode-side
// participation — phase-19 spans BOTH sides).
//
// # Dual-mode processor envelope (ADR-0168)
//
// The `ExternalProcessor` proto carries TWO top-level optional fields —
// `grpc_service` (#1) and `http_service` (#20) — with operator-side
// mutual-exclusion (NOT a proto `oneof`, distinct from phase-18.x ext_authz's
// `services` oneof). 19.1 activates BOTH transport arms:
//
//   - `grpc_service` — opens a bidi-streaming `Process` RPC against the
//     processor cluster via the NEW `*grpcclient.ProcessorClient` wrapper
//     (ADR-0169 — EXTENDS phase-18.2 ADR-0158 `internal/grpcclient/Dialer`;
//     no `Dialer` API changes). One stream per HTTP transaction; per-stage
//     `ProcessingRequest` Send + `ProcessingResponse` Recv; `CloseSend()` on
//     stream end OR `ImmediateResponse` arrival OR `OnDestroy`.
//
//   - `http_service` — POSTs a JSON-encoded `ProcessingRequest` to the
//     processor's `path_prefix` per stage; parses the JSON response into a
//     `ProcessingResponse`. JSON codec is filter-local at `json.go` per
//     ADR-0170 (`protojson` with `UseProtoNames: true` + `EmitUnpopulated:
//     false` + `UseEnumNumbers: false` + `DiscardUnknown: true`). The proto
//     constraint at `ExtProcHttpService.http_service` doc-comment forces
//     body/trailer modes to NONE/SKIP when http_service is set — PARSE-REJECT
//     enforced at config-load time.
//
// Both arms produce a mode-agnostic `processorClient` interface that the
// per-stage dispatcher in `check.go` consumes via the converged
// `applyProcessingResponse` value path.
//
// # 19.1-consumed ExternalProcessor fields (per parent SPEC §5.P1 + SPEC §1)
//
// Top-level (ExternalProcessor proto, 22 fields at go-control-plane v1.32.4):
//
//	Field 1  — grpc_service (*core.v3.GrpcService; GoogleGrpc PARSE-REJECT inherited from ADR-0157)
//	Field 2  — failure_mode_allow (bool; default false)
//	Field 3  — processing_mode (*ProcessingMode; header-modes consumed, body PARSE-REJECT in 19.1, trailer PARSE-REJECT permanently)
//	Field 5  — request_attributes ([]string; CEL attribute allowlist for request_headers stage)
//	Field 6  — response_attributes ([]string; CEL attribute allowlist for response_headers stage)
//	Field 7  — message_timeout (*durationpb.Duration; default 200ms)
//	Field 8  — stat_prefix (string)
//	Field 9  — mutation_rules (*v31.HeaderMutationRules; pre-compiled allow/deny matchers)
//	Field 10 — max_message_timeout (*durationpb.Duration; default 0 = override-disabled)
//	Field 11 — disable_clear_route_cache (bool; mutually exclusive with #18 per §5.P5)
//	Field 12 — forward_rules (*HeaderForwardingRules; allowed/disallowed header matchers)
//	Field 14 — allow_mode_override (bool)
//	Field 15 — disable_immediate_response (bool)
//	Field 18 — route_cache_action (enum; translates #11=true to RETAIN)
//	Field 20 — http_service (*ExtProcHttpService)
//	Field 22 — allowed_override_modes ([]*ProcessingMode)
//
// PARSE-REJECT envoy-go-strict (per parent §5.P10 — STREAMED-only flags
// permanently out of envelope):
//
//	Field 17 — observability_mode (when true)
//	Field 21 — send_body_without_waiting_for_header_response (when true)
//	Field 19 — deferred_close_timeout (when non-zero; observability_mode-coupled)
//
// SILENT-IGNORED top-level fields (deferred per the inline-deferral
// discipline, auditable in the ADR-0040 trail):
//
//	Field 13 — filter_metadata (*structpb.Struct; dynamic-metadata family)
//	Field 16 — metadata_options (dynamic-metadata family)
//	plus core.GrpcService.{initial_metadata, retry_policy}
//
// # Filter shape: BOTH-DECODE-AND-ENCODE (ADR-0167)
//
// ext_proc participates on BOTH sides — decoder for request_headers stage;
// encoder for response_headers stage. The factory's HTTPFilter value has
// `Decoder: f` AND `Encoder: f` (SAME *filter instance), mirroring phase-14
// compressor's both-sides shape but with both sides STRUCTURALLY active in
// 19.1 (compressor's decode side is pass-through; ext_proc's decode side
// fires the request_headers processor call). The compile-time assertions
// `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` +
// `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` enforce this.
//
// Phase 19 is the FIRST §9 row whose deny-path can fire from the ENCODE side
// — `ImmediateResponse` at the response_headers stage triggers
// `f.ecb.SendLocalReply(...)` rather than the customary `f.dcb.SendLocalReply`.
// The framework's `SendLocalReply` primitive per ADR-0085 supports this since
// it enters the encode chain at filter[len-1] per ADR-0075.
//
// # Per-route: 5th-canonical REUSE (NO ADR-0125 §(xiv) amendment — ADR-0173)
//
// `ExtProcPerRoute` carries a PGV-required `override` oneof with two arms:
//
//   - `disabled` (bool, PGV `const: true`): wholly deactivates the filter on
//     this route. envoy-go PARSE-REJECTs `disabled: false`. Per parent SPEC §6
//     amendment 7: a disabled route is wholly inactive — no processor call, no
//     counter increments, no header mutation.
//
//   - `overrides` (*ExtProcOverrides): a NARROWER per-route override carrying
//     7 fields at go-control-plane v1.32.4. MVP-CONSUMED in 19.1: #1
//     `processing_mode` (per-route ProcessingMode override) + #5 `grpc_service`
//     (per-route service override — useful for routing different paths to
//     different processor backends). SILENT-IGNORED per the `[#not-implemented-hide:]`
//     proto-doc convention: #2 `async_mode`, #3 `request_attributes` (distinct
//     from the TOP-LEVEL `ExternalProcessor.request_attributes` #5 which IS
//     MVP-consumed for the listener-level envelope), #4 `response_attributes`,
//     #6 `metadata_options`, #7 `grpc_initial_metadata`.
//
// This maps onto ADR-0125's 5th canonical (disabled-bool arm + narrower
// override sub-message arm in a oneof). Phase 19 lands NO ADR-0125 amendment
// paragraph — the SECOND CONSECUTIVE §9 row after phase 18 to REUSE rather
// than extend. ADR-0173 records the explicit no-amendment classification +
// strengthens the ADR-0125 roster-not-monotonic lesson.
//
// Per-route stats are SHARED with listener-level (mirrors phase-12/13/14/17/18
// SHARED-stats; DIVERGES from phase-11/15/16 INDEPENDENT-stats). The per-route
// override adjusts `processing_mode`/`grpc_service` but still hits the same
// external processor — it spawns no new stateful policy-evaluation surface.
// Cache-on-first-use per parent §5.P7 (per-route config resolved at
// DecodeHeaders time stays in effect for the entire bidi-stream's lifetime,
// even across `ClearRouteCache` invocations).
//
// # 9-counter filterStats (per parent SPEC §6 amendment 4 + ADR-0167)
//
// All 9 counters registered unconditionally at New() time (predeclared empty
// counters for scrape stability per Prometheus best practice; mirrors
// phase-17 jwt_authn + phase-18 ext_authz unconditional-allocation
// discipline). Namespace shape per SN2-reuse: `http.<HCM_stat_prefix>.ext_proc.<counter>`.
//
//   - streams_started: incremented when a bidi-stream is opened (gRPC mode) OR
//     a per-stage POST is dispatched (HTTP mode).
//   - stream_msgs_sent: each ProcessingRequest sent on the stream / per stage.
//   - stream_msgs_received: each ProcessingResponse received.
//   - spurious_msgs_received: ProcessingResponses that do not match the
//     expected stage OR carry CONTINUE_AND_REPLACE in 19.1 (lifts at 19.2 per
//     ADR-0172 §Decision AMENDMENT) OR are ImmediateResponse when
//     disable_immediate_response=true OR carry rejected mutations (one
//     increment per stage with any rejection).
//   - streams_failed: transport-error / timeout / unmarshal-failure / protocol
//     violations.
//   - streams_closed: normal closure (CloseSend after final stage, OR
//     OnDestroy cancellation, OR ImmediateResponse arrival).
//   - failure_mode_allowed: error disposition + failure_mode_allow=true; both
//     streams_failed AND failure_mode_allowed increment.
//   - override_message_timeout_received: ProcessingResponse.override_message_timeout
//     arrived AND was honored.
//   - override_message_timeout_ignored: override_message_timeout out-of-range,
//     OR more than once per stage, OR max_message_timeout=0 (override disabled).
//
// # Bidi-stream lifecycle + async-resume (per parent §5.P10 + ADR-0167)
//
// gRPC mode: one `Process` stream per HTTP transaction. Opened at
// DecodeHeaders entry when request_header_mode ∈ {SEND, DEFAULT}; per-stage
// ProcessingRequest Send + ProcessingResponse Recv park the dispatch
// goroutine on a per-stream resume channel (phase-09 async-resume primitive
// reuse on BOTH decode and encode sides — encode-side async-resume is a
// 19.1 FIRST). The stream's `ctx` carries the per-stream cancellation hook
// so `OnDestroy` aborts in-flight `Send`/`Recv` calls; `CloseSend()` signals
// end-of-stream from the client side.
//
// HTTP mode: NO stream — each stage is a one-shot POST. The async-resume
// discipline still applies (the HTTP `client.Do` blocks; the dispatch
// goroutine parks on the resume channel).
//
// # ADR anchors (per ADR-0044 ADR-on-impl convention)
//
//   - ADR-0167 (Task 2): package shape + BOTH-DECODE-AND-ENCODE filter +
//     9-counter filterStats + unconditional allocation + multi-stage
//     SendLocalReply mechanism + boot-registration alphabetical between
//     extauthz and fault + the TWELFTH §9 row framing +
//     FIRST-cross-phase-consumer-of-ADR-0158/0165/0166 framing.
//   - ADR-0168 (Task 2 §Decision draft; Task 11 §Consequences): compiledConfig
//     shape + grpc_service-vs-http_service mutually-exclusive top-level field
//     dispatch + processorClient interface + http_service proto-constraint +
//     19.1 body-mode PARSE-REJECT + STREAMED-only flag PARSE-REJECT +
//     error-posture fields + GoogleGrpc PARSE-REJECT inherited from ADR-0157
//     §Decision AMENDMENT + initial_metadata/retry_policy SILENT-IGNORE.
//   - ADR-0169 (Task 4): *ProcessorClient bidi-stream wrapper EXTENDING
//     internal/grpcclient/Dialer (ADR-0158 §Consequences anchored this
//     cross-phase shape). FIRST cross-phase consumer of ADR-0158 outside
//     phase-18.2 itself.
//   - ADR-0170 (Task 6): filter-local protojson codec for http_service mode.
//   - ADR-0171 (Task 7, header-mode portion; §Decision AMENDED at 19.2 for
//     body-mode): ProcessingMode state machine + mode-override discipline +
//     override_message_timeout API enablement + per-stage timer-reset.
//   - ADR-0172 (Task 8, header-mode portion; §Decision AMENDED at 19.2 for
//     body-mutation + body-stage immediate_response): CommonResponse mutation
//   - ImmediateResponse multi-stage deny discipline + mutation_rules per-
//     header gating + clear_route_cache + route_cache_action precedence +
//     gRPC-downstream-detection + CONTINUE_AND_REPLACE 19.1 spurious classification.
//   - ADR-0173 (Task 10): per-route 5th-canonical REUSE classification (NO
//     ADR-0125 §(xiv) amendment) + SHARED-stats + 9-counter stat surface +
//     cache-on-first-use + PGV wrinkles.
//   - ADR-0174 (Task 5): symmetric EncoderFilterCallbacks 6-method extension
//     mirroring ADR-0165's decode-side accessors. Required for
//     response_attributes envelope population at the response_headers stage.
//   - ADR-0175 lands at 19.2 (encode-side body-buffering primitive).
//   - ADR-0176 (parent SPEC commit, FULL body): ADR-0045 split-application
//     ADR — UNCHANGED by 19.1 IMPL.
//
// # Divergence windows vs reference Envoy v1.37.2
//
//   - Body-mode arms (request_body_mode / response_body_mode != NONE):
//     PARSE-REJECT in 19.1; lifts at 19.2.
//   - Trailer-mode arms (request_trailer_mode / response_trailer_mode !=
//     SKIP): PARSE-REJECT permanently per §5.P10 RATIFIED.
//   - STREAMED-only flags (observability_mode / send_body_without_waiting_for_header_response
//     / deferred_close_timeout): PARSE-REJECT permanently per §5.P10
//     RATIFIED.
//   - core.GrpcService.GoogleGrpc arm: PARSE-REJECT per ADR-0157 §Decision
//     AMENDMENT inherited from phase-18.2.
//   - core.GrpcService.{initial_metadata, retry_policy}: SILENT-IGNORED per
//     ADR-0168 §Decision.
//   - Dynamic-metadata family (filter_metadata + metadata_options + per-route
//     metadata_options): DEFERRED per the consistent §9-row inline-deferral
//     discipline.
//   - response_code_details on deny: NOT emitted (joint divergence-window
//     with phase-16 rbac + phase-17 jwt_authn + phase-18 ext_authz per parent
//     §5.P12).
//
// # Public API
//
//   - TypeURL: canonical Envoy type-URL for the ext_proc HTTP filter config.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error):
//     the HTTPFilterFactory registered at boot per ADR-0072 (boot-registration
//     site in cmd/envoy-go/main.go alphabetical between `extauthz` and `fault`
//     per ADR-0100 §2.2).
package extproc
