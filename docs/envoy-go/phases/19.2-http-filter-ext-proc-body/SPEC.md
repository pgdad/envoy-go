# Phase 19.2 SPEC — `envoy.filters.http.ext_proc` (body-mode extension + `EncoderFilterCallbacks.BufferEncodedBody` framework primitive)

> **Lifecycle state:** SPEC.md authored; this commit flips ROADMAP row `19.2` `planned → in-progress` (parent row `19` stays `in-progress`; row `19.1` stays `done`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md`. This SPEC is the authoritative input to the 19.2 PLAN.

**Parent:** `docs/envoy-go/phases/19-http-filter-ext-proc/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4, the 13-pin empirical-pin block §5, the §6 amendment block, the 10-ADR anchor map §7, the §3 ADR-0045 split rationale + §8 parent-rollup discipline + §11 19.2 scope subset). This 19.2 SPEC details the body-mode surface only; it REFERENCES the parent's §4/§5/§7/§8/§11 rather than repeating them.

**Sibling predecessor:** `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/SPEC.md` (the foundational filter scaffold + headers-stages-only delivery; `done` at `95bb425` per phase-19.1 REVIEW.md §4 Gate E). The 8 anchored ADRs (ADR-0167..ADR-0174 — package shape + 9-counter `filterStats` + `compiledConfig` + `ProcessorClient` bidi-stream + JSON codec + `ProcessingMode` state machine header-portion + `CommonResponse`/ImmediateResponse header-portion + per-route 5th-canonical REUSE + symmetric `EncoderFilterCallbacks`) ALL LAND AT 19.1 and are REUSED unchanged at 19.2 — 19.2 supplies the body-stage extension of the 8-ADR envelope + ships the NEW `EncoderFilterCallbacks.BufferEncodedBody` framework primitive (ADR-0175). This SPEC supersedes `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/README.md` (the sibling stub authored at the parent SPEC commit).

**Sub-phase scope (per parent SPEC §2 + §11):** 19.2 lands `envoy.filters.http.ext_proc` body-stage participation — activates the `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arms in the existing `internal/filter/http/extproc/` package's `compiledConfig` dispatch (replacing the 19.1 PARSE-REJECT; ADR-0168 §Decision AMENDMENT); lands the NEW `EncoderFilterCallbacks.BufferEncodedBody` framework primitive (ADR-0175 — envoy-go's FIRST encode-side body-buffering primitive, analogous to phase-13 ADR-0128 decode-side `BufferedBody`); the body-stage `ProcessingMode` state-machine extension (ADR-0171 §Decision AMENDMENT — 4-stage at-most-once-per-stage discipline; `mode_override` per parent §5.P1 RATIFIED-AND-REFINED stays header-response-paths-only); body-stage response-mapping (`body_mutation` oneof body/clear_body — `streamed_response` PARSE-REJECT; `status = CONTINUE_AND_REPLACE`; body-stage `ImmediateResponse`; ADR-0172 §Decision AMENDMENT); differential fixture `0023-http-ext-proc-body` (6 scenarios); the 25th fuzzer `FuzzProcessingResponseMapping`.

**ADR continuity:** Phase 19.1 closed at ADR-0174 (the 8 phase-19.1-landing ADRs ADR-0167..ADR-0174 §Decision + §Consequences all landed at their per-Task Lands-in-Tasks; D12 hypothesis HELD per `STATE.md`). 19.2-landing ADRs are **ADR-0175 (§Decision + §Consequences; §Context already anchored at the parent SPEC commit `9cc1458` per ADR-0044), ADR-0168 §Decision AMENDMENT (body-mode PARSE-REJECT lift; in-place per ADR-0044), ADR-0171 §Decision AMENDMENT (body-stage state-machine extension; in-place per ADR-0044), ADR-0172 §Decision AMENDMENT (`body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse` activation; in-place per ADR-0044)**. ADR-0044 escape-valve held in reserve for ~0–1 impl-time-unanticipated ADRs (the parent §5 13-pin block is CLOSED — 7 RATIFIED + 1 RATIFIED-AND-REFINED + 3 RATIFIED-AT-19.1-IMPL-TIME + 2 REFUTED-and-fired; the §5.P11 REFUTATION is the load-bearing trigger that fires ADR-0175 at 19.2). Next-free ADR is **ADR-0177** (UNCHANGED from 19.1 IMPL; ADR-0175 was reserved at the parent SPEC commit; the 3 AMENDMENTS edit existing ADR bodies in-place per ADR-0044).

**Authored:** 2026-05-16.

---

## 1. Purpose

Phase 19.2 lands `envoy.filters.http.ext_proc` in **body-stage mode (BUFFERED)** — the body-mode extension landing — completing the phase-19 ADR-0045 split. The 19.1 envelope (8 anchored ADRs ADR-0167..ADR-0174 + the package skeleton + the bidi-stream + the JSON codec + the 9-counter `filterStats` + the per-route + the 2-stage `ProcessingMode` state machine + the header-stage `CommonResponse`/ImmediateResponse + the encoder-side callback symmetry) is REUSED unchanged at 19.2; 19.2 supplies the body-stage extension + the NEW encode-side body-buffering framework primitive. The four architectural primitives:

1. **NEW `EncoderFilterCallbacks.BufferEncodedBody` framework primitive (ADR-0175).** envoy-go's FIRST encode-side body-buffering primitive — the symmetric mirror of phase-13 ADR-0128 decode-side `BufferedBody` (which the request_body stage REUSES unchanged). Per parent §5.P11 REFUTED at SPEC time: the existing encode-side `DataStopIterationAndBuffer` chain disposition is **park-only** (does NOT accumulate body bytes across `EncodeData` calls); the phase-14 ADR-0131 `EncoderFilterCallbacks.OverwriteBody` primitive is **per-call replacement only** (NOT buffer-and-hold). The NEW primitive: (a) extends `internal/filter/http/callbacks.go`'s `EncoderFilterCallbacks` interface with a `BufferEncodedBody() []byte` accessor returning the accumulated buffered body bytes; (b) extends `internal/filter/http/chain.go`'s `RunEncodeData` to ACCUMULATE bytes on `DataStopIterationAndBuffer` (mirroring `RunDecodeData`'s existing accumulation discipline) into a per-encoderCB `encodeBuf []byte` field; (c) on resume via `ContinueEncoding()` the accumulated-and-possibly-mutated body releases downstream + `encodeBuf` clears. Cross-phase-reusable for any future filter needing encode-side full-body inspection/mutation (e.g., a future encode-side compressor variant beyond phase-14's per-call `OverwriteBody` pattern). Estimate ~150–300 LoC chain-side + ~30–50 LoC interface extension + ~30–50 LoC reader implementation + ~150–250 LoC unit tests.

2. **`request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arm activation (ADR-0168 §Decision AMENDMENT).** The body-mode arms in `buildCompiledConfig` (`extproc.go`) — which 19.1 PARSE-REJECTed with `"ext_proc: request_body_mode != NONE not yet supported (lands in phase 19.2)"` and analog response-direction — now ACCEPT-AND-WIRE the BUFFERED arm. The `compiledConfig` struct's field shape is UNCHANGED from 19.1 (per ADR-0168 §Decision (xi): "the mode-agnostic `compiledConfig` struct is field-final at 19.1; body-mode-specific runtime state lives in closure captures inside `processFn`") — 19.2 only changes (i) the body-mode PARSE-REJECT guards to ACCEPT-AND-WIRE for `BUFFERED`, (ii) the `processFn` closure-capture envelope (the per-stream body-buffer pointers + the body-stage dispatch flags). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT envoy-go-strict permanently (per parent §4.4 deferred-field discipline + §11 19.2 scope subset — "STREAMED-class body modes PARSE-REJECT permanently"). For HTTP-service mode the body-mode PARSE-REJECT continues unchanged (HTTP-service-mode is headers-only per ADR-0168 §Decision; see §2 item 1).

3. **Body-stage `ProcessingMode` state-machine extension (ADR-0171 §Decision AMENDMENT).** The 19.1 per-direction `ProcessingMode` state machine — `stageRequestHeaders` + `stageResponseHeaders` (numStages 2) with at-most-once-per-stage discipline + `mode_override` per parent §5.P1 RATIFIED-AND-REFINED (header-response paths ONLY; propagates to subsequent stages) — extends with `stageRequestBody` + `stageResponseBody` (numStages 4). The at-most-once-per-stage discipline EXTENDS unchanged (each of the 4 stages emits at most one outbound `ProcessingRequest` per stream); the per-direction state mutates as the stream progresses (decode side: headers → body → end; encode side: headers → body → end). `mode_override` continues IGNORED on body-response paths (per parent §5.P1 RATIFINED-AND-REFINED — silently dropped, NOT counted as `spurious_msgs_received`; the REFINEMENT is unchanged at 19.2). The `applyProcessingResponseFn` per-stage dispatch table (the 19.1 5-value `actContinue`/`actStop`/`actError`/`actImmediate`/`actContinueButStillWaiting` enum) is REUSED unchanged; body-stage dispatch produces the same action set.

4. **Body-stage `CommonResponse.body_mutation` + `status = CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse` (ADR-0172 §Decision AMENDMENT).** The 19.1 7-step `applyProcessingResponse` dispatcher (`ImmediateResponse` → `override_message_timeout` → `mode_override` → `CommonResponse` → applyHeaderMutation → `clear_route_cache` → `CONTINUE_AND_REPLACE`) is REUSED unchanged; 19.2 ACTIVATES three previously-spurious arms:
   - **`body_mutation` (`*BodyMutation` oneof)**: `body []byte` arm replaces the buffered body bytes; `clear_body bool` arm replaces with empty bytes (semantics: zero-byte body forwarded downstream + `Content-Length: 0` reconciliation per ADR-0128 §Decision post-body Content-Length reconciliation discipline — REUSED unchanged on the decode side and APPLIED FRESH on the encode side via ADR-0175 §Decision); `streamed_response *StreamedBodyResponse` arm PARSE-REJECTs (STREAMED is out-of-envelope permanently — the IMPL emits `streamed_response not supported (STREAMED body modes out-of-envelope per parent §4.4)` as the `spurious_msgs_received` increment reason; treated as a malformed response per ADR-0172 §Decision (iv) discipline).
   - **`status = CONTINUE_AND_REPLACE`**: at header stages WHEN body-mode is BUFFERED, supersedes both `header_mutation` + `body_mutation` with combined-replacement semantics (the entire request/response is replaced — headers per `header_mutation`, body per `body_mutation` — and the body-stage outbound call is SKIPPED since the body is supplied by the header-stage response). At body stages, `CONTINUE_AND_REPLACE` is treated as `CONTINUE` (per the proto's "ignored at body stages" wording — the body is already in flight; replacement would race with the buffered-body release path). The 19.1 SPEC §12 deferred decision #7 (`CONTINUE_AND_REPLACE` handling) is SETTLED at 19.2 SPEC per this clause.
   - **Body-stage `ImmediateResponse`**: ImmediateResponse can now fire at `request_body` + `response_body` stages in addition to the 19.1-landing header stages. The 4-stage `SendLocalReply` mechanism (the 19.1 multi-stage deny-path infrastructure) covers body-stage dispatch unchanged — the deny-path wire shape is identical at all four stages (per parent §5.P11 RATIFIED-AND-REFINED — emit `gRPC-Status` HEADER not trailer; gRPC-downstream via content-type sniff per ADR-0172 §Decision). The `clear_route_cache` field continues IGNORED at body stages (per the proto's "ignored in the response direction" wording — applies equally to body-stage response_headers paths).

---

## 2. Non-purposes

(Explicit list of envelope-exclusions for 19.2; mirrors 19.1 SPEC §2 + adds 19.2-specific items. Each item carries forward into §8 deferred-items where applicable.)

1. **HTTP-service mode body activation.** HTTP-service mode stays headers-only per ADR-0168 §Decision (http_service proto-constraint: PARSE-REJECT body/trailer modes when `http_service` is set). The body-mode PARSE-REJECT lift at item 2 of §1 applies ONLY to the gRPC-service arm; HTTP-service-mode body PARSE-REJECT continues. This is not a deferral — it's an envoy-go-strict + proto-faithful exclusion.
2. **STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes.** Permanent PARSE-REJECT per parent §4.4 deferred-field discipline + §11 19.2 scope subset. The IMPL emits per-mode PARSE-REJECT errors naming the unsupported arm. Not a deferral.
3. **Trailer modes (anything other than SKIP).** Permanent PARSE-REJECT per 19.1 SPEC §2 + parent §4.4. Body-mode does not lift trailer-mode restrictions; `request_trailer_mode != SKIP` and `response_trailer_mode != SKIP` continue PARSE-REJECT.
4. **`observability_mode` + `send_body_without_waiting_for_header_response` + `deferred_close_timeout`.** Permanent PARSE-REJECT (STREAMED-class flags per ADR-0171 §Decision; carry-forward from 19.1 SPEC §2).
5. **8 reference-Envoy additional counters NOT in the MVP 9-counter set.** Per ADR-0173 §Consequences, reference Envoy emits 8 additional counters (e.g., `immediate_responses_sent`, `message_timeouts`, `streams_canceled`, ...) not in envoy-go's 9-counter MVP set. **19.2 HOLDS at the 9-counter set + 86-stat-name BEHAVIOR_CONTRACT total.** Counter activation deferred to a future phase (19.3+); ADR-0173 §Decision stays unmodified.
6. **3 ADR-0170 §Consequences envelope-content divergences.** (a) protojson whitespace non-determinism; (b) reference Envoy emits `metadata_context:{}` + `protocol_config:{}` as empty messages where envoy-go omits; (c) writer-side `value`-vs-`raw_value` on header injection. **All three remain DEFERRED at 19.2** — body-mode mechanics are orthogonal to envelope-content rendering; closure deferred to a future phase. ADR-0170 §Consequences stays unmodified.
7. **`subject_local_certificate` TLS-fixture closure (Carryforward M from 19.1 REVIEW §10).** **Reassigned to the TLS-fixture phase, NOT 19.2.** Body-mode is orthogonal to TLS attribute population; the 6 encode-side accessors (ADR-0174) already cover the wire shape. The TLS-fronted processor-cluster fixture infrastructure work belongs to the TLS-fixture phase. §8 carries forward.
8. **Dynamic-metadata family** (`metadata_options`, `filter_metadata`, `metadata_context` emit, `CommonResponse.dynamic_metadata`, `CommonResponse.trailers`). Permanent deferral per 19.1 SPEC §2 + parent §4.4. Body-mode does not lift.
9. **`response_code_details` emission.** Joint divergence with phases 16+17+18 forward-pointers (carry-forward from 19.1 SPEC §2 item 15). 19.2 does not lift.
10. **`mode_override` body-response-path responsiveness.** Per parent §5.P1 RATIFIED-AND-REFINED — `mode_override` on body-stage responses is IGNORED (silently dropped, not counted as spurious). The REFINEMENT stays unchanged at 19.2; body-mode does NOT change `mode_override`'s envelope.
11. **Per-message timer enforcement.** Per ADR-0171 §Decision (vi) cross-ref to ADR-0172 §Decision (vii) — at 19.1 IMPL the per-message timer was STRUCTURAL-ONLY (NO code enforcement; deferred to 19.2 IMPL with explicit Task 12 D9-discipline review). 19.2 lifts to behavioral enforcement at IMPL (the 19.1 SPEC §12 item 6 deferred decision is SETTLED via this clause — `context.WithTimeout` cancel-and-rebuild per the parent §5.P5 reference-Envoy convention). NOT a deferral; lands at 19.2 IMPL.
12. **`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` per-route override fields.** Permanent deferral (19.1 SPEC §2 item 12). 19.2 does not lift.
13. **`core.GrpcService.GoogleGrpc`** — envoy-go-strict exclusion (NOT a deferral; ADR-0008 + ADR-0168 §Decision discipline).
14. **xDS-CDS-driven processor-cluster reconfig.** envoy-go has no xDS-CDS (carry-forward from 19.1 SPEC §2 + 18.2 SPEC §8 item 9).
15. **`request_attributes`/`response_attributes` CEL-attribute-name exact roster.** IMPL-settle per 19.1 SPEC §12 item 9. 19.2 IMPL extends the response_body + request_body attribute envelope along the same axis (body-stage attributes mirror header-stage attributes per ADR-0170 §Consequences); the exact CEL-attribute-name roster stays IMPL-settle.

---

## 3. Framework survey result (ONE new framework primitive in 19.2 + FOUR reuses from 19.1 + earlier phases)

19.2 lands ONE new framework primitive + reuses the 19.1-landing primitives unchanged + reuses the phase-13 decode-side body-buffering primitive unchanged. Per parent §4 cross-cutting + §11 19.2 scope subset:

### 3.1 NEW: `EncoderFilterCallbacks.BufferEncodedBody` (ADR-0175)

The symmetric mirror of phase-13 ADR-0128 `BufferedBody` decode-side primitive. Surface:

- **Interface extension** at `internal/filter/http/callbacks.go`'s `EncoderFilterCallbacks`: `BufferEncodedBody() []byte` returns the accumulated buffered body bytes (analogous to `BufferedBody()` on `DecoderFilterCallbacks`). NO new chain-field plumbing on the per-encoderCB struct beyond a `encodeBuf []byte` field SET inside `RunEncodeData` accumulation.
- **Chain-side extension** at `internal/filter/http/chain.go`'s `RunEncodeData`: when the filter returns `DataStopIterationAndBuffer`, accumulate the incoming chunk into the per-encoderCB `encodeBuf` AND do NOT forward downstream (mirrors the existing decode-side accumulation discipline). When the filter calls `ContinueEncoding()` later, forward the (possibly-mutated) accumulated buffer + clear it. The end-of-stream signal (the last `EncodeData` call with `end_stream=true` per the existing chain dispatch convention) closes the buffering window — the filter MUST resume by end-of-stream or the response is treated as truncated per the existing chain-side error path.
- **Mutation interaction**: when the filter resumes via `ContinueEncoding()` after a body-mutation outbound call (e.g., the processor returned `body_mutation{body: newBytes}`), the filter SHOULD have already replaced the buffer contents (the filter's job, not the chain's). The chain releases whatever is in `encodeBuf` at resume time. The encode-side analog of ADR-0128 §Decision's post-body Content-Length reconciliation applies — when the body length changes, the filter (NOT the chain) is responsible for updating `Content-Length` on the response headers via the existing `EncoderFilterCallbacks` header-mutation API.
- **Cross-phase reuse intent**: any future encode-side filter needing full-body inspection or mutation can REUSE the primitive without re-deriving the buffer-and-hold discipline. Specifically named in ADR-0175 §Decision: an encode-side body-transformation filter (e.g., a hypothetical encode-side `lua` filter body callback or an encode-side content-injection filter) could consume the primitive without additional chain-side work.

### 3.2 REUSE: phase-13 ADR-0128 decode-side body-buffering (`DecoderFilterCallbacks.BufferedBody`)

The 19.2 request_body stage REUSES ADR-0128's decode-side primitive unchanged. Per parent §5.P11 (the SPEC-time scrape of master tip confirmed the decode-side primitive intact + functioning per phase-13's design):

- `DecoderFilterCallbacks.BufferedBody() []byte` returns the accumulated request body bytes.
- The chain's `RunDecodeData` already accumulates on `DataStopIterationAndBuffer` + clears on `ContinueDecoding`.
- Post-body Content-Length reconciliation (ADR-0128 §Decision) handles the header-vs-struct-field divergence when the request body length changes — APPLIED FRESH on the encode side via ADR-0175 §Decision.

NO modification to ADR-0128 at 19.2; the primitive is consumed as-is.

### 3.3 REUSE: 19.1-landing primitives (8-ADR envelope ADR-0167..ADR-0174)

All 8 primitives REUSED unchanged at 19.2 — only `processFn` closure-capture envelope extends + body-stage dispatch arms in the existing per-stage table fill in:

- **ADR-0167** package shape + 9-counter `filterStats` + boot-registration + multi-stage `SendLocalReply` + BOTH-decode-and-encode HTTPFilter: REUSED. 19.2 does NOT add new files to the package (body-stage logic extends existing `processor.go` + `check.go` + `attributes.go`); does NOT add new counters; does NOT change boot-registration.
- **ADR-0168** `compiledConfig` shape + grpc_service/http_service dispatch + STREAMED-only flag PARSE-REJECT: REUSED. 19.2 §Decision AMENDMENT lifts the body-mode PARSE-REJECT for gRPC-service-mode BUFFERED; STREAMED-only flag PARSE-REJECT continues unchanged. The `compiledConfig` struct gains NO new fields (per ADR-0168 §Decision (xi)).
- **ADR-0169** `ProcessorClient` bidi-stream wrapper at `internal/grpcclient/processor_client.go`: REUSED. 19.2 does NOT change `ProcessStream`'s 3-method interface (Send/Recv/CloseSend); the body-stage outbound `ProcessingRequest` (carrying `HttpBody`) reuses the same `Send()` invocation as the header-stage outbound (carrying `HttpHeaders`).
- **ADR-0170** ProcessingRequest/ProcessingResponse JSON codec at `extproc/json.go`: REUSED. 19.2 does NOT change protojson options; the body-stage `ProcessingRequest`/`ProcessingResponse` envelopes route through the same `marshalProcessingRequest`/`unmarshalProcessingResponse` pair. The 3 envelope-content divergences from ADR-0170 §Consequences stay DEFERRED per §2 item 6.
- **ADR-0171** `ProcessingMode` state-machine + mode-override: REUSED + 19.2 §Decision AMENDMENT extends numStages 2 → 4. The at-most-once-per-stage discipline + `mode_override` header-response-paths-only refinement carry forward unchanged.
- **ADR-0172** `CommonResponse` mutation + ImmediateResponse multi-stage deny: REUSED + 19.2 §Decision AMENDMENT activates `body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse`. The 7-step `applyProcessingResponse` dispatcher carries forward unchanged.
- **ADR-0173** per-route 5th-canonical REUSE + SHARED-stats: REUSED. 19.2 does NOT amend ADR-0125; per-route `ExtProcOverrides.processing_mode` body-mode arms are now CONSUMED for the gRPC-service arm (per item 2 of §1).
- **ADR-0174** symmetric `EncoderFilterCallbacks` extension (6 socket/TLS/listener accessors): REUSED. 19.2 does NOT add new accessors beyond the 7th NEW `BufferEncodedBody` accessor from ADR-0175. The 6 ADR-0174 accessors populate `response_attributes` at the response_body stage exactly as at the response_headers stage (the chain-fields are SET-once at HCM dispatch + survive across stage transitions).

### 3.4 NOT-REUSED: phase-14 ADR-0131 `EncoderFilterCallbacks.OverwriteBody`

Explicitly NOT reused at 19.2 per parent §5.P11 + ADR-0175 §Context. `OverwriteBody(b []byte)` is per-call replacement (the chain forwards `b` downstream immediately) — distinct from buffer-and-hold semantics needed for body-mode BUFFERED's "ship buffered body to processor, await response, apply mutation, release". The two primitives are COMPLEMENTARY (a future encode-side filter may use either): `OverwriteBody` for per-call replacement (e.g., phase-14 compressor); `BufferEncodedBody` for full-body inspection/mutation (ADR-0175). No conflict; both stay on `EncoderFilterCallbacks` post-19.2.

---

## 4. Body-stage wire shape

### 4.1 Body-stage outbound: `ProcessingRequest{request_body}` + `ProcessingRequest{response_body}`

Per parent §5.P5 RATIFIED + the proto contract: when `request_body_mode = BUFFERED` is configured, after `DecodeHeaders` completes its outbound `ProcessingRequest{request_headers}` round-trip and receives the (possibly-mutated) headers, the filter accumulates the request body across `DecodeData` calls (via ADR-0128 decode-side `BufferedBody`) until end_stream. At end_stream, the filter emits a `ProcessingRequest{request_body: HttpBody{body: <accumulated>, end_of_stream: true}}` on the EXISTING bidi-stream (no new stream opened); parks the decode goroutine on the resume channel; awaits the `ProcessingResponse{request_body: BodyResponse{response: CommonResponse{...}}}` from the processor. The response is applied per the 7-step `applyProcessingResponse` dispatcher with `body_mutation` ACTIVATED (per item 4 of §1). The encode side mirrors symmetrically using the NEW ADR-0175 `BufferEncodedBody` accumulation.

The body-stage outbound `ProcessingRequest` populates a small attribute subset (per ADR-0170 §Consequences body-stage extension): the same envelope as the corresponding header stage's attributes minus envelope fields that no longer apply at body time (e.g., `request.size` is the actual body size at body stage, not the Content-Length-derived header-time size). Exact attribute roster stays IMPL-settle (per §2 item 15).

### 4.2 Body-stage response: `body_mutation` oneof discipline

`ProcessingResponse.{request_body, response_body}.BodyResponse.response.CommonResponse.body_mutation` is `*BodyMutation`:

| Oneof arm | 19.2 disposition |
|---|---|
| `body []byte` | **CONSUMED.** Replaces the buffered body bytes with the processor-supplied bytes. The filter mutates the buffer in place (via the per-stream body-buffer pointer captured in the `processFn` closure), then resumes via `ContinueDecoding()` (decode side) or `ContinueEncoding()` (encode side). Post-body Content-Length reconciliation per ADR-0128 §Decision (decode side) and ADR-0175 §Decision (encode side) — the filter updates `Content-Length` on the corresponding header set via the existing callback header-mutation API before resume. |
| `clear_body bool` (true) | **CONSUMED.** Replaces the buffered body bytes with an empty slice. Same `Content-Length: 0` reconciliation discipline as the `body` arm. `clear_body: false` is treated as no-op (the buffer is unchanged). |
| `streamed_response *StreamedBodyResponse` | **PARSE-REJECT.** STREAMED body modes are out-of-envelope permanently (per §2 item 2 + parent §4.4). The IMPL increments `spurious_msgs_received` + treats as a malformed response per ADR-0172 §Decision (iv) discipline. |

### 4.3 Body-stage `CommonResponse.status = CONTINUE_AND_REPLACE`

| Stage where emitted | 19.2 disposition |
|---|---|
| Header stages (request_headers, response_headers) WITH body-mode = NONE | **CONSUMED as a no-op for body** (no body to replace; behaves as plain `CONTINUE` for the body axis — `header_mutation` still applies per ADR-0172 §Decision header-portion). |
| Header stages WITH body-mode = BUFFERED | **CONSUMED as combined header+body replacement.** `header_mutation` AND `body_mutation` both apply; the corresponding body-stage outbound call is SKIPPED (the body is supplied by the header-stage response). State machine transitions: emit `actContinueButStillWaiting` from the header-stage dispatcher → on body accumulation completion, skip body-stage dispatch → emit `actContinue` after applying mutations. |
| Body stages (request_body, response_body) | **TREATED AS CONTINUE** (per the proto's "ignored at body stages" wording + the race-avoidance rationale in §1 item 4). No counter increment (NOT spurious — the proto silently ignores). The 19.1 spurious-dispatch for `CONTINUE_AND_REPLACE` at header stages with body-mode = NONE (per 19.1 ADR-0172 §Decision (vi)) LIFTS at 19.2 — the disposition becomes "CONSUMED as no-op for body" per the first row above. |

The 19.1 SPEC §12 deferred decision #7 (`CONTINUE_AND_REPLACE` handling) is SETTLED at 19.2 SPEC per this table.

### 4.4 Body-stage `ImmediateResponse` (multi-stage deny extension)

The 19.1 multi-stage `SendLocalReply` infrastructure (the 4-stage deny-path wire shape) is REUSED unchanged for body-stage dispatch. An `ImmediateResponse` emitted from the processor at `ProcessingResponse{request_body}` or `ProcessingResponse{response_body}` fires `SendLocalReply` from the corresponding decode/encode-side path with the proto-specified status + headers + body + (optional) grpc_status. Per ADR-0172 §Decision: gRPC-status emits as a HEADER (not trailer) — the 19.1 SPEC-divergence record stays unchanged at 19.2; gRPC-downstream detection via content-type sniff per ADR-0172 §Decision.

`clear_route_cache` at body-stage `ImmediateResponse` continues IGNORED (per the proto's "ignored in the response direction" wording — applies to body-stage response_headers paths and ALSO to body-stage emission because route cache is moot once the upstream call has begun).

### 4.5 4-stage state-machine extension (`numStages` 2 → 4)

The 19.1 per-direction `ProcessingMode` state machine extends with two new stages — `stageRequestBody` + `stageResponseBody` — bringing total `numStages` from 2 to 4. The at-most-once-per-stage discipline + the 5-value action enum (`actContinue`/`actStop`/`actError`/`actImmediate`/`actContinueButStillWaiting`) are REUSED unchanged. The per-stage dispatch transitions:

```
decode side:
  stageRequestHeaders → stageRequestBody (if request_body_mode = BUFFERED)
                     → done           (if request_body_mode = NONE)
  stageRequestBody    → done

encode side:
  stageResponseHeaders → stageResponseBody (if response_body_mode = BUFFERED)
                       → done           (if response_body_mode = NONE)
  stageResponseBody    → done
```

`mode_override` per parent §5.P1 RATIFIED-AND-REFINED: mid-stream from a body-stage response, IGNORED (silently dropped — per the REFINEMENT, only header-response paths apply `mode_override`). From a header-stage response, `mode_override` continues to validate against `allowed_override_modes` + can alter the body_mode for subsequent stages — IF the header-stage response arrives before the body-stage outbound call fires.

---

## 5. Per-route discipline — 5th-canonical REUSE UNCHANGED; `ExtProcOverrides.processing_mode` body-mode arms now consumed

The 19.1 ADR-0173 §Decision (per-route 5th-canonical REUSE — disabled-bool arm + NARROWER override sub-message in oneof; NO ADR-0125 amendment; SHARED-stats; cache-on-first-use per §5.P7) is REUSED unchanged at 19.2. No new per-route canonical pattern lands.

`ExtProcOverrides.processing_mode` body-mode arms (`request_body_mode`, `response_body_mode`) become CONSUMED at 19.2 (paralleling the listener-level activation per item 2 of §1). Per-route override semantics are unchanged: the per-route override REPLACES the listener-level `processing_mode` field-by-field for the listener+route merge per the proto-faithful map-merge convention. Cache-on-first-use (resolved at `DecodeHeaders` stays in effect for stream lifetime per ADR-0173 §Consequences) is UNCHANGED — the body-stage dispatch reads the cached `compiledConfig` from filter state without re-resolving.

`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` continue SILENT-IGNORED (§2 item 12 carry-forward from 19.1 SPEC §2).

SHARED-stats UNCHANGED — body-mode dispatch does not add new stateful policy-evaluation surface; the 9-counter envelope continues to count at the listener level only (no per-route stat split per ADR-0173 §Consequences).

---

## 6. compiledConfig + code shapes

### 6.1 Public surface UNCHANGED from 19.1

The `internal/filter/http/extproc/` package's public surface (the `Factory()` constructor returning the registered `envoyhttp.HTTPFilter`) is UNCHANGED at 19.2. No new exported types; no new exported functions; no new build-tag.

### 6.2 `compiledConfig` struct UNCHANGED

Per ADR-0168 §Decision (xi): "the mode-agnostic `compiledConfig` struct is field-final at 19.1; body-mode-specific runtime state lives in closure captures inside `processFn`." 19.2 adheres — the struct gains NO new fields. Body-mode-specific runtime state (the per-stream body-buffer pointer + the body-stage dispatch flags + the body-stage stage-machine pointer) lives in the `processFn` closure scope, captured at filter-factory time from the resolved `compiledConfig`.

### 6.3 `DecodeData` + `EncodeData` body-stage dispatch pseudocode

**DecodeData (body-stage activation when `request_body_mode = BUFFERED`):**

```
DecodeData(buf, endStream) StatusType:
  if !cfg.bodyModeActive(request) { return Continue }      // headers-only path (19.1 envelope)
  if !endStream:
    accumulate(decodeBuf, buf)                              // ADR-0128 reuse
    return StopIterationAndBuffer
  // endStream == true: emit body-stage outbound
  accumulated := callbacks.BufferedBody()                   // ADR-0128 reuse
  pr := buildProcessingRequest_RequestBody(accumulated)     // attribute envelope per ADR-0170 + 4.1
  send pr on bidi-stream (ADR-0169 ProcessorClient.Send)
  park decode goroutine on resume channel
  // resume via applyProcessingResponse on RECV from processor
  return StopIterationAndBuffer
```

**EncodeData (body-stage activation when `response_body_mode = BUFFERED`):**

```
EncodeData(buf, endStream) StatusType:
  if !cfg.bodyModeActive(response) { return Continue }     // headers-only path (19.1 envelope)
  if !endStream:
    accumulate(encodeBuf, buf)                              // ADR-0175 NEW
    return StopIterationAndBuffer
  // endStream == true: emit body-stage outbound
  accumulated := callbacks.BufferEncodedBody()              // ADR-0175 NEW
  pr := buildProcessingRequest_ResponseBody(accumulated)    // attribute envelope per ADR-0170 + 4.1
  send pr on bidi-stream (ADR-0169 ProcessorClient.Send)
  park encode goroutine on resume channel
  // resume via applyProcessingResponse on RECV from processor
  return StopIterationAndBuffer
```

The resume path applies the 7-step `applyProcessingResponse` per ADR-0172 §Decision (REUSED unchanged from 19.1; `body_mutation` arm + body-stage `ImmediateResponse` arm + `CONTINUE_AND_REPLACE` arm now ACTIVE per item 4 of §1).

### 6.4 4-stage state-machine code shape

The 19.1 `activeProcessingMode` per-direction state struct extends with body-stage tracking. The `processFn` closure captures (or directly references) the per-direction state to drive the 4-stage at-most-once-per-stage discipline. Pseudocode (abbreviated):

```
type activeProcessingMode struct {
    decode struct {
        stage       stageEnum   // {stageRequestHeaders, stageRequestBody, stageDone}
        bodyMode    enum        // {NONE, BUFFERED}
        bodyBuf     []byte      // per-stream body buffer (decode side: via ADR-0128)
    }
    encode struct {
        stage       stageEnum   // {stageResponseHeaders, stageResponseBody, stageDone}
        bodyMode    enum        // {NONE, BUFFERED}
        bodyBuf     []byte      // per-stream body buffer (encode side: via ADR-0175)
    }
}
```

The exact field layout stays IMPL-settle (the planner may consolidate decode + encode into a single 4-stage `stage` enum if cleaner; the 19.1 split-by-direction precedent suggests keeping them separate). The at-most-once-per-stage guard checks the `stage` enum against the expected stage on each callback entry; spurious entries (e.g., a second `DecodeHeaders` call on the same stream) increment `spurious_msgs_received` per the existing 19.1 discipline.

### 6.5 File layout — NO new files

The 19.2 IMPL extends EXISTING files; no new files in `internal/filter/http/extproc/`:

| File | 19.1 contents | 19.2 extension |
|---|---|---|
| `extproc.go` | `Factory()`, `buildCompiledConfig` dispatch, body-mode PARSE-REJECT guards | body-mode PARSE-REJECT guards lift for BUFFERED gRPC-service-mode (ADR-0168 §Decision AMENDMENT) |
| `processor.go` | 2-stage state machine, processFn closure, header-stage dispatch | 4-stage state machine, body-stage dispatch arms in processFn (ADR-0171 §Decision AMENDMENT) |
| `check.go` | `applyProcessingResponse` 7-step dispatcher (header-portion of arms) | body-stage activation of `body_mutation` / `CONTINUE_AND_REPLACE` / body-stage ImmediateResponse arms (ADR-0172 §Decision AMENDMENT) |
| `attributes.go` | header-stage attribute envelope builder | body-stage attribute envelope builder (request_body + response_body attribute subsets per ADR-0170 §Consequences) |
| `json.go` | protojson marshal/unmarshal | UNCHANGED |
| `route.go` (or analog) | per-route 5th-canonical | `ExtProcOverrides.processing_mode` body-mode arms become consumed |
| `extproc_test.go` | unit tests | body-stage unit tests (new test groups) |
| `fuzz_test.go` | `FuzzExtProcConfigParse` (24th fuzzer) | NEW `FuzzProcessingResponseMapping` (25th fuzzer) per §7.3 |

Cross-package file extension at `internal/filter/http/`:

| File | 19.1 contents | 19.2 extension |
|---|---|---|
| `callbacks.go` | `EncoderFilterCallbacks` with 6 ADR-0174 accessors | NEW `BufferEncodedBody() []byte` accessor (ADR-0175 §Decision) |
| `chain.go` | `RunEncodeData` with park-only `DataStopIterationAndBuffer` | NEW accumulation discipline mirroring `RunDecodeData` (ADR-0175 §Decision) |
| `chain_test.go` | encoder-side callback symmetry tests | NEW body-buffering accumulation + resume tests (ADR-0175 §Consequences) |

### 6.6 Cross-cutting field-level changes

The `processFn` closure capture envelope extends from 19.1's (compiledConfig pointer + bidi-stream handle + per-stream state pointer + 2-stage dispatch table) to include the per-stream body-buffer pointers (decode + encode) + the 4-stage dispatch table. The exact closure-capture layout stays IMPL-settle (the planner may capture by value vs by pointer per the existing 19.1 ADR-0168 §Decision (xi) discipline).

---

## 7. Differential fixture `0023-http-ext-proc-body`

### 7.1 Fixture-author convention

Per the existing differential-fixture-author convention (phases 09..19.1 precedent + 19.1 ADR-0167 §Consequences fixture-numbering): `0023-http-ext-proc-body/` adds a new fixture directory at `test/fixtures/0023-http-ext-proc-body/` with the standard YAML config + per-scenario assertion files. The fixture REUSES the 19.1 `test/helpers/extprocgrpc/` test-helper UNMODIFIED (per parent §11 19.2 scope subset — the same bidi-stream gRPC `Process` server; 19.2 adds new scripted response sequences for body-stage scenarios).

### 7.2 Scenario list (6 scenarios per parent §11 19.2 scope subset)

| # | Name | What it exercises | Assertion focus |
|---|---|---|---|
| (a) | `request_body_buffered_mutation` | request_body BUFFERED + processor sets `body_mutation{body: newBytes}` | byte-exact upstream-arrival assertion (the upstream sees the mutated body bytes); `Content-Length` reconciled on upstream-arrival headers |
| (b) | `response_body_buffered_mutation` | response_body BUFFERED + processor sets `body_mutation{body: newBytes}` | byte-exact downstream-arrival assertion (the curl client sees the mutated body bytes); `Content-Length` reconciled on downstream-arrival headers |
| (c) | `body_stage_immediate_response` | response_body BUFFERED + processor emits `ImmediateResponse{status: 403, body: "denied"}` at body stage | byte-exact downstream 403 status + body; gRPC-status emitted as HEADER per ADR-0172; `failure_mode_allow` does NOT fire (immediate is intentional) |
| (d) | `body_stage_clear_body` | response_body BUFFERED + processor sets `body_mutation{clear_body: true}` | downstream sees zero-byte body + `Content-Length: 0` reconciled |
| (e) | `header_stage_continue_and_replace` | response_headers stage WITH response_body_mode = BUFFERED + processor emits `CommonResponse{status: CONTINUE_AND_REPLACE, header_mutation: {...}, body_mutation: {body: newBytes}}` | combined header+body replacement; body-stage outbound call SKIPPED (no body-stage `streams_messages_sent` increment); downstream sees mutated headers + body |
| (f) | `per_route_body_mode_override` | per-route override sets `request_body_mode = BUFFERED` for one route only; default route stays headers-only | route-A exercises body-stage dispatch + counter increments; route-B exercises 19.1-style headers-only dispatch; counter-delta differences per-route are observable (PRESENCE-check per ADR-0173 §Consequences) |

### 7.3 The 25th fuzzer `FuzzProcessingResponseMapping`

NEW fuzzer at `internal/filter/http/extproc/fuzz_test.go` (extending the 19.1-landing 24th fuzzer file). Surface:

- Input: arbitrary protobytes interpreted as `*extprocv3.ProcessingResponse`.
- Process: `proto.Unmarshal` (catches malformed bytes — should never panic) → `applyProcessingResponse` dispatch (catches malformed-but-valid-proto inputs; e.g., a `CommonResponse` with both `header_mutation` and `streamed_response`).
- Invariants: never panics; never blocks (the dispatch returns a stage-dispatch-action in bounded time); spurious increments stay bounded (no infinite spurious-loop).
- Corpus seeds: 6+ seeds covering the body-stage CommonResponse + body_mutation + CONTINUE_AND_REPLACE arms (named explicitly so the corpus exercises the new 19.2 arms, not just the 19.1 header-portion).
- Budget: 30s ADR-0018 per-fuzzer budget (existing 24 fuzzers continue green at 30s).
- Test layout: parallel to the 19.1-landing `FuzzExtProcConfigParse` — same file, separate `Fuzz...` function.

The existing 24 fuzzers continue green at 30s each at 19.2 IMPL phase-done; the 25th fuzzer enters the gate per `BOOTSTRAP_PROMPT.md` §7.5 Gate D.

### 7.4 Three-listener topology REUSED

The 19.1 fixture-0022 three-listener topology (l_test_a/b/c) is REUSED at 0023 (per the 18.2 / 19.1 fixture-precedent). The processor cluster is shared across listeners; the bidi-stream gRPC `Process` server (the `test/helpers/extprocgrpc/` test-helper) is shared; scenario-specific response sequences load via the standard scenario-discriminator pattern (the helper dispatches on `:path` per the 19.1 precedent).

### 7.5 Counter-delta assertion stays at PRESENCE-check

Per ADR-0173 §Consequences AMENDMENT at 19.1 Task 13: counter-delta assertions stay at PRESENCE-check (counter EXISTS + counter VALUE > 0) NOT strict-equivalence. 19.2 maintains the PRESENCE-check discipline (the 19.1 Open Follow-up #4 "per-scenario counter-delta strict equivalence" carries forward as a 19.2 deferred item — see §8 item 4).

---

## 8. Deferred items (19.2 slice; carry-forward from 19.1 SPEC §8 + 19.1 REVIEW §10 + new 19.2-specific)

For future-phase consideration (none are blockers for closing rows 19.2 + 19; all auditable in the ADR-0040 deferral trail). 19.2 carries forward 17 items from 19.1's §8 list (item 1 closes for body-mode arms; remaining 17 carry forward) + adds 6 new 19.2-specific deferred items:

1. **19.1 §8 item 1 — body-stage activation (`request_body_mode`/`response_body_mode` BUFFERED):** **CLOSED at 19.2** (this SPEC lifts the PARSE-REJECT for the gRPC-service arm; HTTP-service-mode body PARSE-REJECT continues per §2 item 1).
2. **19.1 §8 item 2 — STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes:** carry-forward as permanent deferral (per §2 item 2).
3. **19.1 §8 item 3 — trailer modes:** carry-forward (per §2 item 3).
4. **19.1 §8 items 4..6 — `observability_mode` + `send_body_without_waiting_for_header_response` + `deferred_close_timeout`:** carry-forward (per §2 item 4).
5. **19.1 §8 items 7..10 — dynamic-metadata family (`metadata_options`, `filter_metadata`, `metadata_context`/`dynamic_metadata` emit, `CommonResponse.dynamic_metadata`+`CommonResponse.trailers`):** carry-forward (per §2 item 8).
6. **19.1 §8 item 11 — `HttpHeaders.attributes` (deprecated):** carry-forward (silent-ignore unchanged).
7. **19.1 §8 item 12 — `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}`:** carry-forward (per §2 item 12).
8. **19.1 §8 item 13 — `core.GrpcService.GoogleGrpc`:** envoy-go-strict exclusion (per §2 item 13).
9. **19.1 §8 item 14 — `core.GrpcService.{initial_metadata, retry_policy}`:** SILENT-IGNORED, carry-forward.
10. **19.1 §8 item 15 — `response_code_details` emission:** carry-forward (per §2 item 9).
11. **19.1 §8 item 16 — xDS-CDS-driven processor-cluster reconfig:** carry-forward (per §2 item 14).
12. **19.1 §8 item 17 — TLS-fronted processor-cluster fixture coverage:** carry-forward (per §2 item 7 — REASSIGNED to TLS-fixture phase).
13. **19.1 §8 item 18 — `request_attributes`/`response_attributes` CEL-attribute-name allowlist exact roster:** IMPL-settle, carry-forward (per §2 item 15).
14. **NEW: 8 reference-Envoy additional counters NOT in MVP 9-counter set** (per ADR-0173 §Consequences). 19.2 holds at 9 counters per §2 item 5; counter activation deferred to a future phase. Stat-table BEHAVIOR_CONTRACT total stays at 86 names.
15. **NEW: 3 ADR-0170 §Consequences envelope-content divergences** (protojson whitespace; reference Envoy empty-message emission; writer-side `value`-vs-`raw_value`). 19.2 defers per §2 item 6; closure deferred to a future phase.
16. **NEW: per-scenario counter-delta strict equivalence** (the 19.1 REVIEW §10 Open Follow-up #4 cross-ref). Stays at PRESENCE-check per §7.5; closure deferred to a future phase pending the strict-equivalence-vs-counter-superset-activation joint decision.
17. **NEW: `applyProcessingResponseFn` package-level indirection refactor** (Carryforward R from 19.1 REVIEW §10). Lower-priority cleanup — move from package-level `var` to a `compiledConfig` or `factoryState` field; permits removing the `t.Parallel` discipline guard. Deferred to a future phase.
18. **NEW: ADR-0175 cross-phase consumption forward-pointer.** Future encode-side body-transformation filters that consume `BufferEncodedBody()` (a hypothetical encode-side lua-callback body filter; an encode-side content-injection filter) recorded as a forward-pointer per ADR-0175 §Consequences. NOT a deferral; a forward-pointer for grep-archaeology.

---

## 9. Cross-references against phase-19.1 deferred-items list — forward-pointer pickup

- **19.1 §8 item 1 — body-stage activation:** **CLOSED at 19.2** (gRPC-service arm only; HTTP-service-mode body PARSE-REJECT continues — split closure per §2 item 1).
- **19.1 §8 items 2..18:** NO PICKUP (carry-forward per §8 items 2..13 above).
- **19.1 §12 deferred decision #7 — `CONTINUE_AND_REPLACE` handling:** **CLOSED at 19.2 SPEC** (per §4.3 table — settled to "CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED; TREATED AS CONTINUE at body stages").
- **19.1 §12 deferred decision #6 — `override_message_timeout` timer reset:** **CLOSED at 19.2 IMPL** (per §2 item 11 — per-message timer enforcement lifts to behavioral via `context.WithTimeout` cancel-and-rebuild per parent §5.P5).
- **19.1 §12 other deferred decisions (#1..#5, #8):** NO PICKUP (carry-forward — these are 19.1-IMPL or 19.1-anchor decisions, not body-mode-related).
- **19.1 REVIEW §10 Forward-pointers to 19.2 (verbatim enumeration):** all 11 enumerated items are CARRIED into this §8 (items 1..18 above subsume them; see in particular §8 item 12 Carryforward M reassignment, §8 item 14 8-counter activation deferral, §8 item 16 counter-delta deferral, §8 item 17 applyProcessingResponseFn refactor deferral).
- **Phase 18.2 forward-pointers + earlier phases:** NO PICKUP (ext_authz-specific or earlier-filter-specific concerns; 19.2 is body-mode-scoped).

**Forward-pointer net change for phase 19.2**: **2 closures** (19.1 §8 item 1 body-stage activation closes for gRPC-service-mode; 19.1 §12 #7 `CONTINUE_AND_REPLACE` handling closes at SPEC). 19.2 adds 5 new deferred items (§8 items 14..18). Net new deferred items: +5; net closures: 2; net deferred-cluster delta: +3 vs 19.1.

---

## 10. ADR anchor map (19.2 subset; full 10-ADR map in parent SPEC §7)

The 19.2-landing ADRs. Per the ADR-0044 ADR-on-impl convention: ADR-0175 §Context was anchored at the parent SPEC commit `9cc1458`; §Decision + §Consequences LAND at the Lands-in-Task per ADR-0044. ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS are IN-PLACE edits to the existing 19.1-anchored §Decision sections at their respective Lands-in-Tasks at 19.2 IMPL (per ADR-0044).

| ADR | Subject (19.2 portion) | Lands-in-Task (hypothesis) |
|---|---|---|
| **ADR-0175** | `EncoderFilterCallbacks.BufferEncodedBody()` framework primitive — encode-side body-buffering analogous to ADR-0128 decode-side; chain-side `RunEncodeData` accumulation extension; per-encoderCB `encodeBuf` field; cross-phase-reusable forward-pointer for future encode-side body-transformation filters | Task T-N (the IMPL task that lands chain.go + callbacks.go + chain_test.go body-buffering extension; PLAN-time settles N) |
| **ADR-0168 §Decision AMENDMENT** | `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arm activation for gRPC-service mode: replace the 19.1 PARSE-REJECT with the body-stage dispatch wire-up; STREAMED-class arms continue PARSE-REJECT permanently; HTTP-service-mode body PARSE-REJECT continues; `compiledConfig` struct field-final invariant preserved (NO new fields) | Task T-N (the IMPL task that lifts extproc.go's body-mode PARSE-REJECT guards) |
| **ADR-0171 §Decision AMENDMENT** | 4-stage state-machine extension: `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline extends to 4 stages; `mode_override` header-response-paths-only refinement carries unchanged; the 5-value action enum unchanged; per-message timer enforcement lifts to behavioral (§2 item 11) | Task T-N (the IMPL task that extends processor.go's state machine) |
| **ADR-0172 §Decision AMENDMENT** | `body_mutation` (body / clear_body arms CONSUMED; streamed_response arm PARSE-REJECT) + `status = CONTINUE_AND_REPLACE` (CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED; TREATED AS CONTINUE at body stages; 19.1 spurious-dispatch lifts) + body-stage `ImmediateResponse` (CONSUMED — multi-stage `SendLocalReply` REUSED unchanged); `clear_route_cache` at body stages continues ignored | Task T-N (the IMPL task that activates body-mode arms of `applyProcessingResponse` in check.go) |

**ADR-0044 escape-valve** held in reserve for ~0–1 impl-time-unanticipated ADRs. The parent §5.P11 REFUTATION + the 19.1-landing 8-ADR envelope are the load-bearing inputs; the 19.2 IMPL surface that COULD fire a new ADR per the parent §11 19.2 scope subset: (i) buffered-body-release-vs-stream-reset interaction with the framework's encode-chain primitive (unlikely — the chain-side accumulation discipline mirrors ADR-0128's decode-side equivalent; ADR-0128 had no analogous race-introduction); (ii) `body_mutation_rules` application to body bytes (unlikely per the proto's mutation_rules doc which is header-specific — body mutations bypass mutation_rules). NEITHER appears load-bearing. **NO ADR-0125 amendment.**

**Next-free ADR after phase 19.2** = **ADR-0177** (UNCHANGED — ADR-0175 was reserved at the parent SPEC commit; the 3 AMENDMENTS edit existing ADR bodies in-place per ADR-0044). The 19.2 IMPL may consume ADR-0177 + ADR-0178 if 19.2-IMPL-unanticipated ADRs fire (most-likely surfaces per the parent §11 19.2 scope subset).

---

## 11. Empirical-pin block reference — parent §5 13-pin block fully CLOSED post-19.1; NO new pins at 19.2 SPEC

The parent §5 13-pin block is **fully closed** post-19.1 phase-done (per STATE.md cross-reference):

- **RATIFIED (7 pins):** §19.P2, §19.P3, §19.P5, §19.P6, §19.P9, §19.P10, §19.P13.
- **RATIFIED-AND-REFINED (1 pin):** §19.P1 (`mode_override` timing — header-response paths only; propagates to subsequent stages). The refinement carries unchanged at 19.2 per §1 item 3 + §4.5.
- **RATIFIED-AT-19.1-IMPL-TIME (3 pins):** §19.P4 (9-counter stat surface RATIFIED-WITH-AMENDMENT — 8 reference counters DEFERRED), §19.P7 (cache-on-first-use RATIFIED-BY-CONSTRUCTION), §19.P8 (JSON codec wire shape RATIFIED on codec-options axis — 3 envelope-content divergences DEFERRED).
- **REFUTED (2 pins):** §19.P11 (encode-side body-buffering primitive — fires ADR-0175 at 19.2 SPEC per this SPEC §1 item 1), §19.P12 (encode-side callbacks lack 6 accessors — fired ADR-0174 at 19.1).

**No new empirical pins are anticipated at 19.2 SPEC.** The body-mode extension composes against the 19.1-anchored envelope + the parent §5 closed block; the 19.2 SPEC IS an in-session settle (the D12 discipline). The 19.2 IMPL is expected to fire ~0 unanticipated ADRs per the parent §11 19.2 scope analysis (the body-mode mechanics are well-specified by the proto + the reference Envoy behavior + the ADR-0128 decode-side precedent).

**Reference to parent §5 for all 13 pins.** The 19.2 SPEC inherits the closed state without re-derivation.

---

## 12. Deferred decisions (the planner / implementer settles these)

1. **`EncoderFilterCallbacks.BufferEncodedBody()` exact method name.** SPEC proposes `BufferEncodedBody() []byte` (mirroring `BufferedBody()` on `DecoderFilterCallbacks`; explicit naming for the encode-side symmetric mirror). The IMPL may choose another name (e.g., `BufferedEncodedBody()`, `EncodedBody()`) if it cleans up callsite readability. ADR-0175 §Decision settles at IMPL.
2. **`processFn` closure-capture layout for body-mode state.** Per ADR-0168 §Decision (xi), body-mode-specific state lives in closure captures inside `processFn`. The exact layout (single struct vs separate variables; pointer vs value semantics) stays IMPL-settle. The 19.1 precedent (`processFn` captures pointer-to-`compiledConfig` + pointer-to-per-stream state) suggests pointer captures for body-buffer pointers.
3. **4-stage state-machine field consolidation.** Per §6.4: the planner may consolidate decode + encode stage tracking into a single 4-stage `stage` enum, or keep them split-by-direction (the 19.1 split-by-direction precedent). The IMPL settles per readability.
4. **Per-message timer behavioral enforcement mechanism.** Per §2 item 11: the timer lifts to behavioral via `context.WithTimeout` cancel-and-rebuild per parent §5.P5. The exact implementation (e.g., timer-per-stage vs single rolling timer; cancellation propagation through the bidi-stream send/recv loop) stays IMPL-settle. The Task 12 D9-discipline review (ratified at 19.1) carries forward as the design constraint.
5. **Body-stage attribute envelope exact roster.** Per parent §11 19.2 scope subset + §4.1: body-stage attributes mirror header-stage attributes minus envelope fields that no longer apply at body time. The exact roster (e.g., does `request.body` populate at the `request_body` stage? Does the `xds.upstream_local_address` populate at body stages?) stays IMPL-settle.
6. **PARSE-REJECT error message text for `streamed_response`.** Per §4.2: the IMPL emits a `spurious_msgs_received` increment + a malformed-response error path. The exact error message text (used for the `streams_failed` log emission + the `spurious_msgs_received` increment reason) stays IMPL-settle.
7. **Body-buffer release ordering on `OnDestroy` after body-stage outbound but before processor RECV.** Per ADR-0169 §Decision OnDestroy discipline (REUSED unchanged): when `OnDestroy` fires during a parked body-stage outbound, the per-stream context cancels + the bidi-stream Send/Recv loop unblocks. The body buffer release on the chain side (decode side via `ContinueDecoding()` or encode side via `ContinueEncoding()`) must NOT fire (the chain dispatch is being torn down). The exact synchronization between OnDestroy + the parked body-stage goroutine stays IMPL-settle; the existing 19.1 D9 race-guard primitive (`f.mu` + `f.done` per ADR-0171 §Decision) is the prerequisite.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at 19.2 phase-done)

1. **§13.1 — `## HTTP filter chain` → `### envoy.filters.http.ext_proc` subsection in-place AMENDMENT.** Flip the 19.1-anchored "body modes — see phase 19.2" forward-pointers to ACTUAL body-mode content. Specifically: (a) `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arms CONSUMED for gRPC-service-mode (replacing the 19.1 PARSE-REJECT description); (b) `CommonResponse.body_mutation` (`body` + `clear_body` arms CONSUMED; `streamed_response` arm PARSE-REJECT); (c) `CommonResponse.status = CONTINUE_AND_REPLACE` (CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED; TREATED AS CONTINUE at body stages); (d) body-stage `ImmediateResponse` (CONSUMED — multi-stage `SendLocalReply` REUSED unchanged); (e) per-route `ExtProcOverrides.processing_mode` body-mode arms CONSUMED for gRPC-service-mode; (f) HTTP-service-mode body PARSE-REJECT continues unchanged.
2. **§13.2 — `## Stat-name mapping` → stat-name table UNCHANGED at 86 names.** The 9 ext_proc counters from 19.1 carry forward unchanged (body-mode does not add new counters per §2 item 5).
3. **§13.3 — `## Equivalence Matrix` → NEW row for fixture `0023-http-ext-proc-body`** with byte-exact body/status assertions (per scenario list in §7.2) + 9-counter PRESENCE-check.
4. **§13.4 — NEW `### Phase 19.2 forward-pointer notes` subsection** covering the §8 deferral list (18 items — 19.1 carry-forwards + new 19.2-specific). 19.2 closes 2 phase-19.1 forward-pointers (body-stage activation for gRPC-service-mode + `CONTINUE_AND_REPLACE` SPEC-time settle).
5. **§13.5 — `## HTTPFilterCallbacks` AMENDMENT.** Add the 7th NEW `BufferEncodedBody() []byte` accessor on `EncoderFilterCallbacks` (the symmetric mirror of `BufferedBody()` on `DecoderFilterCallbacks`); cross-phase reuse intent for future encode-side body-transformation filters; ADR-0175 anchor reference. The 6 ADR-0174 accessors stay documented unchanged.
6. **§13.6 — `## Per-route canonical patterns cross-reference` UNCHANGED.** The 5th-canonical REUSE note from 19.1 covers body-mode (ADR-0173 §Decision UNCHANGED at 19.2; per §5).
7. **`## ext_proc framework primitive (per phase 19 ADR-0167 + phase 19.2 ADR-0175)` umbrella AMENDMENT.** Extend the 19.1-anchored ext_proc umbrella with the NEW `BufferEncodedBody` accessor + the encode-side body-buffering chain-side discipline + the cross-phase reuse intent for future encode-side body-transformation filters. The 19.1-anchored bidi-stream `ProcessorClient` primitive section + the JSON codec section + the 9-counter `filterStats` section + the 5th-canonical REUSE section all stay UNCHANGED.
8. **`## gRPC client framework primitive (per phase 18.2 ADR-0158)` UNCHANGED.** The phase-18.2 umbrella stays unchanged; 19.2 does not extend `internal/grpcclient/` (the ADR-0169 `ProcessorClient` wrapper is REUSED unchanged).

---

## 14. Testing strategy

### 14.1 Unit tests

- `internal/filter/http/extproc/` — NEW test groups for body-stage dispatch:
  - **Group N (NEW): body-mode PARSE-REJECT lift** — confirm BUFFERED arms ACCEPT-AND-WIRE; STREAMED arms continue PARSE-REJECT; HTTP-service-mode body PARSE-REJECT continues.
  - **Group N+1 (NEW): body-stage `applyProcessingResponse` dispatch** — `body_mutation{body}` replaces buffer; `body_mutation{clear_body}` empties buffer; `body_mutation{streamed_response}` PARSE-REJECT + spurious_msgs_received increment; `CONTINUE_AND_REPLACE` at header stages with body-mode BUFFERED combines header+body replacement; `CONTINUE_AND_REPLACE` at body stages treated as CONTINUE; body-stage `ImmediateResponse` fires `SendLocalReply` at correct stage.
  - **Group N+2 (NEW): 4-stage state-machine extension** — at-most-once-per-stage discipline extends to 4 stages; spurious entries increment `spurious_msgs_received`; `mode_override` on body-stage responses silently IGNORED.
  - **Group N+3 (NEW): body-stage attribute envelope** — request_body + response_body attribute subsets populate per §4.1.
- `internal/filter/http/` — NEW test groups for chain-side body-buffering:
  - **chain_test.go Group N+4 (NEW): `RunEncodeData` accumulation discipline** — `DataStopIterationAndBuffer` accumulates bytes across multiple `EncodeData` calls; `ContinueEncoding()` releases (possibly-mutated) buffer + clears; end-of-stream signal closes the buffering window; truncated stream (no resume by end_stream) error path matches the decode-side precedent.
  - **callbacks_test.go Group N+5 (NEW): `EncoderFilterCallbacks.BufferEncodedBody()` accessor** — returns accumulated bytes; concurrent-access correctness (race-detector exercises the field access against in-flight `EncodeData` calls).
- Existing 19.1 unit test groups UNCHANGED.

### 14.2 Race detector + lint

`go test -race ./internal/filter/http/extproc/... ./internal/filter/http/...` + repo-wide race clean. The body-stage dispatch path adds new race surfaces:

- **OnDestroy during in-flight body-stage outbound.** Same primitive 19.1 ratified for header-stage outbound: `f.mu`/`f.done` guard + `context.WithCancel`; the body-stage Send/Recv loop honors `ctx.Done()` and returns promptly. The 19.2 IMPL adds `TestOnDestroy_CancelsInFlightBodyStageOutbound` parallel to 19.1's header-stage equivalent.
- **Chain-side `encodeBuf` accumulation concurrent with `ContinueEncoding()`.** The chain dispatches `RunEncodeData` accumulation on the encode goroutine; `ContinueEncoding()` may be called from another goroutine (per the existing 19.1 async-resume pattern). The race detector must exercise this — `chain_test.go` Group N+4 adds the test.
- **Per-message timer behavioral enforcement (§2 item 11).** The `context.WithTimeout` cancel-and-rebuild discipline introduces a new cancellation surface; the race detector exercises the rebuild path against in-flight Send/Recv.

### 14.3 Fuzzer

25th fuzzer `FuzzProcessingResponseMapping` per §7.3. Existing 24 fuzzers re-run clean.

### 14.4 h2spec + differential

h2spec 53/53 PASS at the ADR-0051 pin (NO H2 wire-shape change between 19.1 and 19.2; ext_proc uses gRPC over H2 to the processor cluster, not to the downstream client). 24 differential fixtures green at 19.2 phase-done (0000–0023; 0023 NEW; 0000–0022 carry-forward).

### 14.5 Six-gate checklist (A/B/C/D/E/F per BOOTSTRAP_PROMPT.md §7.5)

- **Gate A** (build + vet + lint): green; `internal/filter/http/extproc/` recompiles clean with body-mode additions; `internal/filter/http/callbacks.go` + `chain.go` recompiles clean with ADR-0175 extension.
- **Gate B** (race tests): green; `go test -race ./internal/filter/http/extproc/... ./internal/filter/http/...` + repo-wide.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin.
- **Gate D** (fuzzers): 25 fuzzers green at 30s each.
- **Gate E** (differential): 24/24 fixtures green (0000–0023).
- **Gate F** (BEHAVIOR_CONTRACT): the §13 edit bundle landed; `tools/check_behavior_contract.sh` (or analog) green; stat-name table stays at 86.

---

## 15. Acceptance checklist (for the reviewer)

The 19.2 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts:

1. **`EncoderFilterCallbacks.BufferEncodedBody() []byte` framework primitive landed per ADR-0175:** interface extension at `internal/filter/http/callbacks.go`; chain-side accumulation discipline at `internal/filter/http/chain.go`'s `RunEncodeData` (mirrors `RunDecodeData`'s ADR-0128 accumulation); per-encoderCB `encodeBuf` field; unit tests at `chain_test.go` exercising accumulation + resume + truncated-stream; cross-phase-reuse forward-pointer in ADR-0175 §Decision.
2. **`request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arm activation for gRPC-service mode per ADR-0168 §Decision AMENDMENT:** body-mode PARSE-REJECT guards in `extproc.go`'s `buildCompiledConfig` lift to ACCEPT-AND-WIRE for BUFFERED; STREAMED-class arms continue PARSE-REJECT permanently; HTTP-service-mode body PARSE-REJECT continues; `compiledConfig` struct field-final invariant preserved (NO new fields per ADR-0168 §Decision (xi)); the 19.1 ADR-0168 §Decision text AMENDED in-place per ADR-0044.
3. **4-stage state-machine extension per ADR-0171 §Decision AMENDMENT:** `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline extends to 4 stages; `mode_override` header-response-paths-only refinement carries unchanged; per-message timer enforcement lifts to behavioral via `context.WithTimeout` cancel-and-rebuild per parent §5.P5; the 19.1 ADR-0171 §Decision text AMENDED in-place per ADR-0044.
4. **Body-stage `body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse` per ADR-0172 §Decision AMENDMENT:** `body_mutation{body}` + `body_mutation{clear_body}` CONSUMED with byte-exact buffer replacement + Content-Length reconciliation; `body_mutation{streamed_response}` PARSE-REJECT + `spurious_msgs_received` increment; `status = CONTINUE_AND_REPLACE` CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED + TREATED AS CONTINUE at body stages; body-stage `ImmediateResponse` fires `SendLocalReply` at correct stage with proto-faithful status/headers/body; `clear_route_cache` at body stages continues IGNORED; the 19.1 ADR-0172 §Decision text AMENDED in-place per ADR-0044.
5. **Per-route discipline UNCHANGED per ADR-0173 §Decision:** 5th-canonical REUSE; SHARED-stats; cache-on-first-use; NO ADR-0125 amendment. `ExtProcOverrides.processing_mode` body-mode arms become CONSUMED for gRPC-service-mode (paralleling the listener-level activation); `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` continue SILENT-IGNORED.
6. **`compiledConfig` struct field-final invariant preserved:** NO new fields added at 19.2 (per ADR-0168 §Decision (xi) + §6.2). Body-mode-specific runtime state lives in closure captures inside `processFn`.
7. **Differential fixture `0023-http-ext-proc-body` per §7:** 6 scenarios (a..f per §7.2) — `request_body_buffered_mutation` + `response_body_buffered_mutation` + `body_stage_immediate_response` + `body_stage_clear_body` + `header_stage_continue_and_replace` + `per_route_body_mode_override`; three-listener topology REUSED from fixture 0022; `test/helpers/extprocgrpc/` test-helper REUSED UNMODIFIED; byte-exact body + status on downstream/upstream-arrival; cross-side counter-delta PRESENCE-check (per ADR-0173 §Consequences AMENDMENT carry-forward).
8. **25th fuzzer `FuzzProcessingResponseMapping` per §7.3:** at `internal/filter/http/extproc/fuzz_test.go`; corpus seeds covering body-stage `CommonResponse` + `body_mutation` + `CONTINUE_AND_REPLACE` arms; 30s ADR-0018 budget; existing 24 fuzzers re-run clean.
9. **Empirical pins:** parent §5 13-pin block CLOSED post-19.1 phase-done (§11). 19.2 SPEC adds NO new pins (D12 discipline); 19.2 IMPL anticipated to fire ~0 unanticipated ADRs.
10. **DECISIONS.md populated** per ADR-on-impl convention: ADR-0175 §Decision + §Consequences landed (§Context was at the parent SPEC commit `9cc1458`); ADR-0168 §Decision AMENDMENT landed in-place; ADR-0171 §Decision AMENDMENT landed in-place; ADR-0172 §Decision AMENDMENT landed in-place. NO new ADR numbers (ADR-0177 remains next-free; if an unanticipated ADR DOES land, it is ADR-0177).
11. **BEHAVIOR_CONTRACT.md populated** per Gate F: §13.1 ext_proc subsection's "body modes — see phase 19.2" forward-pointers flipped to substantive body-mode content (per §13 item 1); §13.3 NEW row for fixture 0023; §13.4 NEW `### Phase 19.2 forward-pointer notes`; §13.5 NEW `BufferEncodedBody` accessor entry on `EncoderFilterCallbacks`; ext_proc umbrella extended with ADR-0175 chain-side discipline note; §13.2 stat-table UNCHANGED at 86 names.
12. **ROADMAP.md** row `19.2` flips `in-progress → done` AT THE SAME COMMIT as parent row `19` flips `in-progress → done` (per parent SPEC §8 parent-rollup discipline). The commit-message body MUST explicitly name BOTH transitions for grep-verifiability.
13. **§9 family-rows discipline preserved per ADR-0106:** §9 HTTP filters family stays flat top-level rows; row `19.2` is a sibling of row `19.1` (NOT nested under row `19`); row `19` parent stays in-progress until 19.2's phase-done; next §9 family-row after 19 closes is numbered `20`.
14. **All six phase-done gates green** at the 19.2 phase-done commit: build/vet/lint clean; race-test clean repo-wide; h2spec 53/53 PASS at the ADR-0051 pin; 25 fuzzers green at 30s each; 24 differential fixtures green (0000–0023); BEHAVIOR_CONTRACT.md populated.
15. **No master mutation outside the 19.2 squash-merge commit** — all work landed on the 19.2 worktree branches per ADR-0005 §Decision 4 + project memory `feedback_git_worktrees.md`; master tip advances only at the squash-merge commit + SHA-fill follow-up.
16. **Carryforward dispositions from 19.1 REVIEW §10 + Task 14:** §8 item 12 reassigns Carryforward M (`subject_local_certificate` TLS-fixture closure) to the TLS-fixture phase per §2 item 7; §8 item 14 keeps the 8 reference-Envoy additional counters DEFERRED (no 19.2 counter-superset activation per §2 item 5); §8 item 15 keeps the 3 ADR-0170 §Consequences envelope-content divergences DEFERRED per §2 item 6; §8 item 16 keeps per-scenario counter-delta strict equivalence DEFERRED (stays at PRESENCE-check per §7.5); §8 item 17 keeps the `applyProcessingResponseFn` package-level indirection refactor DEFERRED (Carryforward R lower-priority cleanup); the 19.1 §12 deferred decisions #6 + #7 close at 19.2 SPEC + 19.2 IMPL per §9.
