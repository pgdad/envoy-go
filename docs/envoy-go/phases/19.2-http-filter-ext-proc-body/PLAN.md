# Phase 19.2 — HTTP filter `envoy.filters.http.ext_proc` (body-mode extension + `EncoderFilterCallbacks.BufferEncodedBody` framework primitive) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.ext_proc` body-stage participation against the 19.1-landing package surface — completing the phase-19 ADR-0045 split. The 19.2 IMPL ships ONE new framework primitive (`EncoderFilterCallbacks.BufferEncodedBody() []byte` per ADR-0175 — envoy-go's FIRST encode-side body-buffering primitive, symmetric mirror of phase-13 ADR-0128 decode-side `BufferedBody`); activates the `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arms in `internal/filter/http/extproc/`'s `compiledConfig` dispatch (ADR-0168 §Decision AMENDMENT — lifts the 19.1 PARSE-REJECT for gRPC-service-mode only; STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED + HTTP-service-mode body PARSE-REJECT permanently); extends the per-direction `ProcessingMode` state machine to 4 stages with at-most-once-per-stage discipline + per-message timer behavioral enforcement (ADR-0171 §Decision AMENDMENT — `numStages` 2 → 4; `mode_override` header-response-paths-only refinement carries unchanged; `context.WithTimeout` cancel-and-rebuild per parent §5.P5); activates body-stage `CommonResponse.body_mutation` (`body` + `clear_body` CONSUMED; `streamed_response` PARSE-REJECT) + `CONTINUE_AND_REPLACE` combined header+body replacement at header stages with body-mode BUFFERED + body-stage `ImmediateResponse` (ADR-0172 §Decision AMENDMENT); differential fixture `0023-http-ext-proc-body` (6 scenarios per SPEC §7.2; REUSES 19.1's `test/helpers/extprocgrpc/` UNMODIFIED); 25th fuzzer `FuzzProcessingResponseMapping` (extends `internal/filter/http/extproc/fuzz_test.go`). **The 19.2 phase-done squash-merge commit closes BOTH row `19.2` AND parent row `19`** per parent SPEC §8 rollup discipline AT THE SAME commit.

**Architecture:** The 19.2 IMPL extends the existing 19.1-landing `internal/filter/http/extproc/` package + the existing `internal/filter/http/{callbacks,chain}.go` framework files in-place — **NO new files in `internal/filter/http/extproc/`** (per SPEC §6.5 — `extproc.go` body-mode PARSE-REJECT guards lift; `processor.go` extends 2-stage → 4-stage state machine; `check.go` activates body-mode arms of `applyProcessingResponse`; `attributes.go` adds body-stage envelope builder; `json.go` UNCHANGED). The NEW `EncoderFilterCallbacks.BufferEncodedBody() []byte` accessor lands at `internal/filter/http/callbacks.go` (ADR-0175 §Decision — symmetric mirror of `BufferedBody()` on `DecoderFilterCallbacks`); the chain-side accumulation discipline lands at `internal/filter/http/chain.go`'s `RunEncodeData` (mirrors `RunDecodeData`'s existing ADR-0128 accumulation — adds an `encodeBuf []byte` field to the per-encoderCB struct, accumulates bytes on `DataStopIterationAndBuffer`, releases on `ContinueEncoding()`, clears + closes the buffering window at end-of-stream). The differential fixture `0023-http-ext-proc-body` REUSES the 19.1 `test/helpers/extprocgrpc/` test-helper UNMODIFIED (per parent §11 19.2 scope subset + SPEC §7.1 — the same bidi-stream gRPC `Process` server; 19.2 adds new scripted response sequences for the 6 body-stage scenarios). The `compiledConfig` struct gains NO new fields per ADR-0168 §Decision (xi) field-final invariant; body-mode-specific runtime state lives in closure captures inside `processFn`. The per-message timer behavioral enforcement (deferred at 19.1 IMPL with explicit Task 12 D9-discipline review per ADR-0171 §Decision (vi) + ADR-0172 §Decision (vii)) lifts to behavioral at 19.2 IMPL per the 19.1 §12 item 6 SPEC settle — single rolling timer per direction via `context.WithTimeout` cancel-and-rebuild (NOT timer-per-stage — at-most-one-in-flight per direction per ADR-0171 §Decision (v) invariant). The 4 anchored ADRs land at their Lands-in-Tasks per ADR-0044 ADR-on-impl convention: ADR-0175 §Decision + §Consequences full bodies (§Context already at parent SPEC commit `9cc1458`); ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS as in-place edits to existing 19.1-anchored §Decision sections. **No new ADR numbers consumed at 19.2 PLAN per D-series hypothesis D10** — next-free `ADR-0177` stays unconsumed; if the IMPL surfaces an unanticipated load-bearing ADR (parent §11 names two unlikely candidates: buffered-body-release-vs-stream-reset interaction with the chain primitive; `body_mutation_rules` application to body bytes), it is `ADR-0177` + PROGRESS.md records the hypothesis as FALSIFIED.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 (proto pin per ADR-0008 — `envoy/extensions/filters/http/ext_proc/v3` + `envoy/service/ext_proc/v3` + `envoy/config/common/mutation_rules/v3` UNCHANGED from 19.1); `google.golang.org/grpc` v1.70.0 (DIRECT dep from phase-18.2 per ADR-0158); `google.golang.org/protobuf/encoding/protojson` (UNCHANGED — 19.2 does NOT modify the JSON codec); `google.golang.org/protobuf/types/known/{durationpb,structpb}` (UNCHANGED); `context.Context` for per-stream + per-message cancellable bidi-stream calls (UNCHANGED — the 19.1 `streamCtx`/`streamCancel` plumbing is REUSED for body-stage dispatch; the per-message rolling timer extends via `context.WithTimeout` cancel-and-rebuild per planner-time D4); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — UNCHANGED); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext h2c processor-cluster fixture (UNCHANGED from 19.1 fixture 0022).

---

## Scope check — why phase 19.2 ships as one row (already the second split half) + 25-task / 1500-LoC gate analysis

Phase 19 was SPLIT into `19.1-http-filter-ext-proc-headers` + `19.2-http-filter-ext-proc-body` at the phase-19 parent SPEC commit (`9cc1458`) per ADR-0045 / ADR-0176; 19.2 is the second sub-phase. No further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward; the 19.2 envelope is too small to motivate one anyway).

**Per ADR-0005 §Decision 4 + SKILL_ROUTING §2** the PLAN gate is **25 tasks** AND **1500 LoC production code**. If either bound is approached, the PLAN must split the phase. The 19.2 envelope:

- **Anticipated task count: 11** (vs the 19.1 PLAN's 15 + the 18.1 PLAN's 15). Well under the 25-task gate.
- **Anticipated production LoC: ~1500–2500** per STATE.md + SPEC §1. The break-down per planner-time D11 estimate:
  - `internal/filter/http/callbacks.go` ADR-0175 extension ~+30–50 LoC (one new interface method).
  - `internal/filter/http/chain.go` ADR-0175 chain-side accumulation discipline ~+150–300 LoC (per-encoderCB `encodeBuf` field + `RunEncodeData` accumulation/release/clear-on-end-stream logic + `BufferEncodedBody` reader method on `*encoderCB`).
  - `internal/filter/http/chain_test.go` body-buffering tests ~+150–250 LoC (accumulation across multiple `EncodeData` calls + resume + clear + truncated-stream + concurrent access).
  - `internal/filter/http/extproc/extproc.go` ADR-0168 §Decision AMENDMENT (PARSE-REJECT lift) + integration ~+100–200 LoC (PARSE-REJECT guard lift for BUFFERED gRPC-service-mode; `compiledConfig` no-new-fields per ADR-0168 §Decision (xi); `DecodeData` + `EncodeData` body-stage dispatch bodies).
  - `internal/filter/http/extproc/processor.go` ADR-0171 §Decision AMENDMENT (4-stage state machine + per-message timer behavioral) ~+200–350 LoC (extends `stage` enum + `activeProcessingMode` per-direction struct with `bodyMode` + `bodyBuf` per planner-time D2 + D3; rolling timer via `context.WithTimeout` cancel-and-rebuild per D4).
  - `internal/filter/http/extproc/check.go` ADR-0172 §Decision AMENDMENT (body-mode arms of `applyProcessingResponse`) ~+150–250 LoC (`body_mutation{body|clear_body}` apply; `streamed_response` PARSE-REJECT per D6; `CONTINUE_AND_REPLACE` combined replacement at header stages with body-mode BUFFERED; body-stage `ImmediateResponse` dispatch via existing 4-stage `SendLocalReply`).
  - `internal/filter/http/extproc/attributes.go` body-stage envelope builder ~+150–250 LoC (`buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest`; attribute roster per planner-time D5 = header-stage SUPERSET).
  - `internal/filter/http/extproc/extproc_test.go` body-stage unit tests ~+600–1000 LoC (Groups N..N+5 per SPEC §14.1).
  - `internal/filter/http/extproc/fuzz_test.go` 25th fuzzer extension ~+100 LoC.
  - `test/fixtures/0023-http-ext-proc-body/` NEW DIRECTORY ~+700–1000 LoC (envoy.yaml + envoy-go.yaml + expectations.yaml + README.md + inputs/driver.go covering 6 scenarios; three-listener topology REUSED from 0022 per SPEC §7.4; `test/helpers/extprocgrpc/` REUSED UNMODIFIED).
  - `test/differential/fixture/fixture.go` + `test/differential/runner_test.go` ~+10 LoC (no new `BackendKind` — fixture 0023 reuses `HTTPExtProcGRPC = 19` from 19.1; only the blank import for `0023/inputs` adds).

  **Production code subtotal: ~830–1400 LoC** (`internal/filter/http/extproc/` ~+450–800 + `internal/filter/http/{callbacks,chain}.go` extension ~+180–350 + boot-reg UNCHANGED). **Comfortably under the 1500-LoC production-code gate.** Total including tests + fixture + docs: ~2400–4000 LoC — significantly smaller than the 19.1 IMPL's 17,105 insertion total.

- **DECISIONS.md changes**: ADR-0175 §Decision + §Consequences full bodies (~+150–250 LoC); 3 in-place §Decision AMENDMENTS for ADR-0168 / ADR-0171 / ADR-0172 (~+50–100 LoC each, totaling ~+150–300 LoC). Total ~+300–550 LoC.
- **BEHAVIOR_CONTRACT.md changes**: SPEC §13's 7-edit bundle (~+200–300 LoC; smaller than 19.1's 8-edit bundle ~+400 LoC because the stat-name table is UNCHANGED at 86 and the ext_proc subsection only ADDS body-mode content rather than authoring it ab initio).
- **ROADMAP.md**: row `19.2` flips `in-progress → done` + parent row `19` flips `in-progress → done` AT THE SAME COMMIT per parent SPEC §8 rollup discipline.
- **STATE.md**: rewrite-in-place per BOOTSTRAP §5.
- **PROGRESS.md + REVIEW.md**: NEW at this phase (~600 LoC PROGRESS across 11 task entries + ~280 LoC REVIEW per the 19.1 precedent).

**Gate verdict: 19.2 ships as the single sub-phase row it is.** No further split. Task-count gate (11 / 25 = 44%) and LoC gate (~1400 / 1500 = 93%) BOTH leave headroom; if mid-IMPL the LoC envelope grows beyond 1500, the task-count cushion absorbs the variance without requiring a re-split (per the phase-13..18.x precedent that LoC-soft-threshold is the load-bearing diagnostic, NOT a hard split trigger; the task-count gate is the canonical gate per SKILL_ROUTING §2).

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/callbacks.go` | MODIFIED | Add ONE new method to `EncoderFilterCallbacks` per ADR-0175 (§5.P11 REFUTED at parent SPEC time): `BufferEncodedBody() []byte` returns the accumulated buffered body bytes (mirrors `BufferedBody()` on `DecoderFilterCallbacks`). Doc-comment cites ADR-0175 + the cross-phase reuse intent (any future encode-side filter needing full-body inspection or mutation — e.g., a hypothetical encode-side `lua` body-callback filter or an encode-side content-injection filter — reuses the primitive without re-deriving the buffer-and-hold discipline). The 6 ADR-0174 accessors stay UNCHANGED. ~+30–50 LoC. Task 2. ADR-0175 §Decision + §Consequences at Task 2. |
| `internal/filter/http/chain.go` | MODIFIED | Add per-encoderCB `encodeBuf []byte` field + extend `RunEncodeData` accumulation discipline per ADR-0175 (mirrors `RunDecodeData`'s existing ADR-0128 accumulation) + add `BufferEncodedBody()` reader method on `*encoderCB` returning the accumulated buffer. The existing `encodeBufLen` running total stays as the size cap (same `filterBufferLimitBytes` enforcement). On `DataStopIterationAndBuffer`: accumulate the chunk into `encodeBuf` AND DO NOT forward downstream (replaces the existing "park-only" comment at chain.go:400-404 per ADR-0175 §Context). On `ContinueEncoding()`: forward the (possibly-mutated) accumulated buffer downstream + clear `encodeBuf` + reset `encodeBufLen` per planner-time D7 (the end-of-stream signal closes the buffering window — the filter MUST resume by end_stream or the response is treated as truncated per the existing chain-side error path; the IMPL settles whether to mirror the decode-side `errDecodeBufferOverflow` symmetrically — D-series D7 hint: SYMMETRIC; the IMPL emits `errEncodeBufferOverflow` + connection reset when the accumulated body exceeds `filterBufferLimitBytes`). ~+150–300 LoC. Task 2. ADR-0175 §Decision + §Consequences at Task 2. |
| `internal/filter/http/chain_test.go` | MODIFIED | Add Group N+4 body-buffering accumulation tests: `TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData` + `TestEncoderCB_BufferEncodedBody_ReturnsAccumulatedBytes` + `TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears` + `TestEncoderCB_EncodeDataEndStream_WithoutResume_ClosesBufferingWindow` + `TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow` (if D7 SYMMETRIC discipline) + `TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding` (per Group N+5 in SPEC §14.1). Mirrors the existing decode-side accumulation tests at `chain_test.go`. ~+150–250 LoC. Task 2. |
| `internal/filter/http/extproc/extproc.go` | MODIFIED | Lift the body-mode PARSE-REJECT guards in `buildCompiledConfig` for `BUFFERED` arms of `request_body_mode` + `response_body_mode` when the service is `grpc_service` (ADR-0168 §Decision AMENDMENT — replaces the 19.1 `"ext_proc: request_body_mode != NONE not yet supported (lands in phase 19.2)"` error with ACCEPT-AND-WIRE). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT permanently per SPEC §2 item 2. HTTP-service-mode body PARSE-REJECT continues unchanged per SPEC §2 item 1. The `compiledConfig` struct gains NO new fields per ADR-0168 §Decision (xi) — body-mode-specific runtime state lives in closure captures inside `processFn` per planner-time D2. `DecodeData` body extends per SPEC §6.3 pseudocode: when `cfg.bodyModeActive(request) && !endStream` → accumulate via ADR-0128 reuse + return `StopIterationAndBuffer`; when `endStream` → emit `ProcessingRequest{request_body}` via Task 5 attribute builder + park decode goroutine on resume channel. `EncodeData` body extends per SPEC §6.3 pseudocode: symmetric using ADR-0175 `BufferEncodedBody()` accumulation. `DecodeTrailers` + `EncodeTrailers` continue pass-through `Continue` (trailer-modes PARSE-REJECT at parse per ADR-0168). ~+100–200 LoC. Task 3 (PARSE-REJECT lift + dispatch sketch) + Task 7 (integration). ADR-0168 §Decision AMENDMENT in-place at Task 3; §Consequences refresh at Task 7 once body-stage dispatch is fully wired. |
| `internal/filter/http/extproc/processor.go` | MODIFIED | Extend the per-direction `ProcessingMode` state machine from 2 stages to 4 (ADR-0171 §Decision AMENDMENT — `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline EXTENDS unchanged). Extend `activeProcessingMode` struct with per-direction `bodyMode BodySendMode` + `bodyBuf []byte` fields per planner-time D2 (single struct, split-by-direction per D3 — the 19.1 split-by-direction precedent preferred over single 4-stage enum). Per-stage dispatch transitions per SPEC §4.5: decode side `stageRequestHeaders → stageRequestBody (if BUFFERED) → done`; encode side `stageResponseHeaders → stageResponseBody (if BUFFERED) → done`. `mode_override` continues IGNORED on body-stage responses per parent §5.P1 RATIFIED-AND-REFINED (header-response paths only; the refinement carries unchanged at 19.2). The 5-value `action` enum (`actContinue`/`actStop`/`actError`/`actImmediate`/`actContinueButStillWaiting`) is REUSED unchanged; body-stage dispatch produces the same action set. **Per-message timer behavioral enforcement** per planner-time D4: single rolling timer per direction (NOT timer-per-stage — at-most-one-in-flight invariant) via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send (replaces the 19.1 structural-only treatment per Carryforward O at 19.1 Task 14). The `override_message_timeout` reset path (19.1 ADR-0171 §Decision (vi)) extends naturally: the existing `handleOverrideMessageTimeout` cancels the in-flight per-message timer + builds a fresh one with the override duration. `OnDestroy` cancels `f.streamCtx` → both decode + encode body-stage Send/Recv goroutines unblock via `ctx.Done()` + return WITHOUT calling `ContinueDecoding`/`ContinueEncoding` per planner-time D7 (the chain teardown releases the buffer naturally; the body-buffer pointers in the per-direction state are reclaimed by GC). ~+200–350 LoC. Task 4. ADR-0171 §Decision AMENDMENT + §Consequences at Task 4. |
| `internal/filter/http/extproc/check.go` | MODIFIED | Activate body-mode arms of `applyProcessingResponse` (ADR-0172 §Decision AMENDMENT). **Switch arms added per SPEC §4.2 + §4.3 + §4.4:** (a) `BodyResponse.response.CommonResponse.body_mutation` switch: `*BodyMutation_Body` → replace the per-direction `bodyBuf` with the processor-supplied bytes + update `Content-Length` on the corresponding header set via the existing callback header-mutation API per ADR-0128 §Decision (decode side) and ADR-0175 §Decision (encode side); `*BodyMutation_ClearBody` (true) → replace `bodyBuf` with empty + reconcile `Content-Length: 0`; `*BodyMutation_ClearBody` (false) → no-op; `*BodyMutation_StreamedResponse` → PARSE-REJECT path: increment `spurious_msgs_received` + emit `streams_failed` log line with the planner-time D6 error text `"ext_proc: streamed_response body mutation not supported (STREAMED body modes out-of-envelope per parent §4.4)"`; classify as malformed-response per ADR-0172 §Decision (iv) discipline. (b) `CommonResponse.status = CONTINUE_AND_REPLACE` at header stages WITH body-mode BUFFERED: combined header+body replacement — `header_mutation` AND `body_mutation` both apply; the corresponding body-stage outbound call SKIPS (state machine emits `actContinueButStillWaiting` from the header-stage dispatcher → on body accumulation completion, skip body-stage dispatch → emit `actContinue` after applying mutations); the 19.1 spurious-dispatch for `CONTINUE_AND_REPLACE` at header stages with body-mode NONE (per 19.1 D7) LIFTS to "CONSUMED as no-op for body" per SPEC §4.3 table. (c) `CommonResponse.status = CONTINUE_AND_REPLACE` at body stages: TREATED AS CONTINUE per the proto's "ignored at body stages" wording; no counter increment. (d) Body-stage `ImmediateResponse`: fires `SendLocalReply` from the corresponding decode/encode-side path via the existing 19.1 multi-stage `SendLocalReply` infrastructure (REUSED unchanged) with the proto-specified status + headers + body + (optional) grpc_status; per ADR-0172 §Decision gRPC-status emits as HEADER (not trailer); gRPC-downstream detection via content-type sniff per ADR-0172 §Decision. (e) `clear_route_cache` at body-stage `ImmediateResponse` continues IGNORED per the proto's "ignored in the response direction" wording. The 7-step dispatcher itself is REUSED unchanged from 19.1; 19.2 ACTIVATES the three previously-spurious arms. ~+150–250 LoC. Task 6. ADR-0172 §Decision AMENDMENT + §Consequences at Task 6. |
| `internal/filter/http/extproc/attributes.go` | MODIFIED | Add body-stage envelope builders: `buildRequestBodyProcessingRequest(f *filter, body []byte, endStream bool, attributeAllowlist []string) *extprocv3.ProcessingRequest` populating `ProcessingRequest.request_body = &HttpBody{body: body, end_of_stream: endStream}` + `ProcessingRequest.attributes` map per planner-time D5 (header-stage SUPERSET — the body-stage roster MIRRORS the header-stage roster MINUS no fields; ADDS body-stage-natural attributes like `request.size` populated accurately from `len(body)` rather than Content-Length-derived; ADDS no envelope fields beyond the existing ADR-0170 + ADR-0174 accessor coverage). Symmetric `buildResponseBodyProcessingRequest(f *filter, body []byte, endStream bool, attributeAllowlist []string) *extprocv3.ProcessingRequest` populating `ProcessingRequest.response_body`. The 19.1-landing header-stage `buildRequestHeadersProcessingRequest` + `buildResponseHeadersProcessingRequest` are UNCHANGED. The CEL-attribute-name → accessor mapping (the SPEC §6.6 hypothesis-table from 19.1) carries forward; the body-stage roster crystallizes at Task 9 fixture-harness scrape per planner-time D5 (the EXACT roster — e.g., whether `request.body`/`response.body` populates at the body stage — is empirical-settle). The existing helpers (`lowercaseHeaderMap`, `sourcePrincipalFirstOrEmpty`) are REUSED unchanged. ~+150–250 LoC. Task 5. Consumes ADR-0170 + ADR-0174 accessor surfaces unchanged. |
| `internal/filter/http/extproc/json.go` | UNCHANGED | 19.2 does NOT modify the JSON codec (per SPEC §3.3 + §6.5 reference). The body-stage `ProcessingRequest`/`ProcessingResponse` envelopes route through the same `marshalProcessingRequest`/`unmarshalProcessingResponse` pair. The 3 envelope-content divergences from ADR-0170 §Consequences stay DEFERRED per SPEC §2 item 6. ~0 LoC net change. |
| `internal/filter/http/extproc/extproc_test.go` | MODIFIED | Add body-stage unit-test groups per SPEC §14.1: **Group N (NEW): body-mode PARSE-REJECT lift** — confirm BUFFERED arms ACCEPT-AND-WIRE; STREAMED arms continue PARSE-REJECT; HTTP-service-mode body PARSE-REJECT continues; mutual-exclusion + per-route override paths. **Group N+1 (NEW): body-stage `applyProcessingResponse` dispatch** — `body_mutation{body}` replaces buffer + Content-Length reconciliation; `body_mutation{clear_body}` empties buffer + Content-Length: 0; `body_mutation{streamed_response}` PARSE-REJECT path increments `spurious_msgs_received` per D6 + emits the D6 error text; `CONTINUE_AND_REPLACE` at header stages with body-mode BUFFERED combines header+body replacement + skips body-stage outbound call; `CONTINUE_AND_REPLACE` at body stages treated as CONTINUE (no counter increment); body-stage `ImmediateResponse` fires `SendLocalReply` at correct stage with proto-faithful status/headers/body/grpc_status. **Group N+2 (NEW): 4-stage state-machine extension** — at-most-once-per-stage discipline extends to 4 stages; spurious entries increment `spurious_msgs_received`; `mode_override` on body-stage responses silently IGNORED (NOT counted as spurious per parent §5.P1 RATIFIED-AND-REFINED). **Group N+3 (NEW): body-stage attribute envelope** — `request.size`/`response.size` populate from `len(body)` at body stage; CEL-attribute-name roster mirrors header-stage per planner-time D5. **Group N+6 (NEW): per-message timer behavioral enforcement** — single rolling timer per direction; `context.WithTimeout` cancel-and-rebuild on each Send; timeout exceeded → `streamsFailed++` + posture per `failure_mode_allow`; `override_message_timeout` reset cancels the in-flight timer + rebuilds with override duration. Existing 19.1 unit-test groups (1-11) STAY UNCHANGED. ~+600–1000 LoC. Tasks 2-6 contribute per-group; Task 7 lands integration tests. |
| `internal/filter/http/extproc/fuzz_test.go` | MODIFIED | Add 25th fuzzer `FuzzProcessingResponseMapping` per SPEC §7.3. Fuzzes arbitrary protobytes → `proto.Unmarshal` of `*extprocv3.ProcessingResponse` → `applyProcessingResponse` dispatch → asserts: never panics; never blocks (dispatch returns in bounded time); spurious increments stay bounded (no infinite spurious-loop). Corpus seeds (6+ variants per SPEC §7.3): `CommonResponse` with `body_mutation{body}`; `body_mutation{clear_body}`; `body_mutation{streamed_response}`; `CONTINUE_AND_REPLACE` with header_mutation + body_mutation combined; body-stage `ImmediateResponse` with grpc_status; malformed `BodyMutation` with both `body` and `clear_body` set. 30s ADR-0018 budget. The existing 24th fuzzer `FuzzExtProcConfigParse` STAYS UNCHANGED. ~+100 LoC. Task 10. |
| `cmd/envoy-go/main.go` | UNCHANGED | 19.2 does NOT touch boot-registration — the `extproc` filter is already registered at the 19.1-anchored `httpReg.Register(extproc.TypeURL, extproc.New)` between `extauthz` and `fault` per ADR-0100 §2.2. |
| `test/helpers/extprocgrpc/` | UNCHANGED | The 19.1 NEW test-helper at `test/helpers/extprocgrpc/{doc.go,extprocgrpc.go,extprocgrpc_test.go}` is REUSED UNMODIFIED per SPEC §7.1 + parent §11 19.2 scope subset. The same `:path`-keyed scriptable per-stage `ProcessingResponse` sequence pattern from 19.1 D1 covers body-stage scenarios. ~0 LoC net change. |
| `test/differential/fixture/fixture.go` | UNCHANGED | The `BackendKind` enum value `HTTPExtProcGRPC = 19` from 19.1 covers fixture 0023 — same backend kind, same processor cluster topology. ~0 LoC net change. |
| `test/differential/runner_test.go` | MODIFIED | Add ONE new blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0023-http-ext-proc-body/inputs"` (alphabetical after `0022`). The switch-case for `HTTPExtProcGRPC` is REUSED unchanged from 19.1. ~+1 LoC. Task 9. |
| `test/fixtures/0023-http-ext-proc-body/` | NEW DIRECTORY | Differential fixture per SPEC §7. **Three-listener topology REUSED** from 0022 (l_test_a/b/c per SPEC §7.4). **6 scenarios per SPEC §7.2**: (a) `request_body_buffered_mutation` — request_body BUFFERED + processor sets `body_mutation{body: newBytes}`; byte-exact upstream-arrival assertion + Content-Length reconciled; (b) `response_body_buffered_mutation` — response_body BUFFERED + processor sets `body_mutation{body: newBytes}`; byte-exact downstream-arrival + Content-Length reconciled; (c) `body_stage_immediate_response` — response_body BUFFERED + processor emits `ImmediateResponse{status: 403, body: "denied"}` at body stage; gRPC-status emits as HEADER per ADR-0172; failure_mode_allow does NOT fire; (d) `body_stage_clear_body` — `body_mutation{clear_body: true}`; zero-byte body downstream + Content-Length: 0; (e) `header_stage_continue_and_replace` — response_headers stage with response_body_mode BUFFERED + processor emits `CommonResponse{status: CONTINUE_AND_REPLACE, header_mutation, body_mutation{body}}`; body-stage outbound SKIPPED (no body-stage `streams_messages_sent` increment); downstream sees mutated headers + body; (f) `per_route_body_mode_override` — per-route override sets `request_body_mode = BUFFERED` for one route only; route-A exercises body-stage dispatch; route-B exercises 19.1-style headers-only dispatch; per-route counter-delta differences observable (PRESENCE-check per ADR-0173 §Consequences AMENDMENT). |
| `test/fixtures/0023-http-ext-proc-body/envoy.yaml` | NEW | Reference Envoy bootstrap. Three-listener topology (l_test_a/b/c REUSED from 0022) + listener-level + per-route `ExternalProcessor` configs activating `request_body_mode = BUFFERED` / `response_body_mode = BUFFERED` per scenario; routes `/scenario_a` through `/scenario_f`; cluster `c_backend` STRICT_DNS → echobackend; cluster `c_ext_proc` STRICT_DNS → extprocgrpc helper (h2c). ~+250 LoC. Task 9. |
| `test/fixtures/0023-http-ext-proc-body/envoy-go.yaml` | NEW | Equivalent envoy-go bootstrap. Same topology + routes + per-route map; cluster type STATIC. ~+250 LoC. Task 9. |
| `test/fixtures/0023-http-ext-proc-body/inputs/driver.go` | NEW | Go driver issuing the 6 scenarios. Functions `runScenarioA..runScenarioF(ctx, baseURLs, processorBaseURL) error`. Per-scenario assertion: byte-exact upstream-arrival body (scenario a) + downstream-arrival body (scenarios b/c/d/e); status equivalence; `/stats/prometheus` counter-delta PRESENCE-check; backend-arrival/downstream-arrival header assertions including reconciled `Content-Length`; processor-server received-`ProcessingRequest` content assertions (per-stage discriminator with body-stage attribute envelope per planner-time D5; scenario e asserts the body-stage outbound call did NOT fire — `streams_messages_sent` increment on the processor side stays at the header-stage-only baseline). The 19.1 driver primitives (`setupProcessorGRPC`, `scrapeStats`, `assertCounterDelta`) are REUSED unchanged. ~+400 LoC. Task 9. |
| `test/fixtures/0023-http-ext-proc-body/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7.2 + §7.5 PRESENCE-check discipline. Documents the 6-scenario equivalence claim + the 3 ADR-0170 §Consequences envelope-content divergences DEFERRED per SPEC §2 item 6 + the 8 reference-Envoy counter activation surface DEFERRED per SPEC §2 item 5 + the per-scenario counter-delta strict equivalence DEFERRED per SPEC §8 item 16. ~+90 LoC. Task 9. |
| `test/fixtures/0023-http-ext-proc-body/README.md` | NEW | Fixture overview + 6-scenario list + reference-config citations + three-listener topology REUSE note (from 0022) + `test/helpers/extprocgrpc/` REUSE-UNMODIFIED note + body-mode dispatch + ADR-0175 chain-side discipline cross-reference + divergence-window note (STREAMED-class arms PARSE-REJECT permanently; HTTP-service-mode body PARSE-REJECT continues; 3 ADR-0170 envelope-content divergences DEFERRED; 8 reference-Envoy counter activation DEFERRED). ~+150 LoC. Task 9. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | **4 ADR landings at 19.2 IMPL** per ADR-0044: **ADR-0175** §Decision + §Consequences full bodies at Task 2 (extends the existing §Context draft at parent SPEC commit `9cc1458`; the §Decision body covers the `BufferEncodedBody()` method name + signature; chain-side accumulation discipline mirroring ADR-0128; `encodeBuf` field + overflow handling per planner-time D7; cross-phase reuse intent for future encode-side body-transformation filters; the SYMMETRIC pattern relative to ADR-0128 + DISTINCT from ADR-0131 `OverwriteBody`); **ADR-0168 §Decision AMENDMENT** in-place at Task 3 (lifts the body-mode PARSE-REJECT for gRPC-service-mode BUFFERED arms; STREAMED-class arms continue PARSE-REJECT permanently; HTTP-service-mode body PARSE-REJECT continues; `compiledConfig` struct field-final invariant preserved per ADR-0168 §Decision (xi)); **ADR-0171 §Decision AMENDMENT + §Consequences** in-place at Task 4 (`numStages` 2 → 4 extension; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline extends; `mode_override` header-response-paths-only refinement carries unchanged; per-message timer behavioral enforcement lifts to behavioral via `context.WithTimeout` cancel-and-rebuild); **ADR-0172 §Decision AMENDMENT + §Consequences** in-place at Task 6 (`body_mutation{body|clear_body}` CONSUMED; `body_mutation{streamed_response}` PARSE-REJECT; `CONTINUE_AND_REPLACE` CONSUMED as combined header+body replacement at header stages with body-mode BUFFERED + TREATED AS CONTINUE at body stages; body-stage `ImmediateResponse` CONSUMED via existing multi-stage `SendLocalReply`; `clear_route_cache` at body stages IGNORED). NO new ADR numbers consumed at 19.2 PLAN (D-series D10 hypothesis). ~+300–550 LoC across Tasks 2/3/4/6. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 — **7-edit bundle**: **§13.1** ext_proc subsection in-place AMENDMENT — flip the 19.1-anchored "body modes — see phase 19.2" forward-pointers to ACTUAL body-mode content (body-mode BUFFERED arms CONSUMED for gRPC-service-mode; `body_mutation` body/clear/streamed_response arms; `CONTINUE_AND_REPLACE` combined replacement at header stages with body-mode BUFFERED + TREATED AS CONTINUE at body stages; body-stage `ImmediateResponse` CONSUMED; per-route body-mode arms CONSUMED for gRPC-service-mode; HTTP-service-mode body PARSE-REJECT continues unchanged). **§13.2** stat-name table UNCHANGED at 86 names (body-mode adds no new counters per SPEC §2 item 5). **§13.3** Equivalence Matrix NEW row for fixture `0023-http-ext-proc-body` with byte-exact body/status assertions + 9-counter PRESENCE-check. **§13.4** NEW `### Phase 19.2 forward-pointer notes` subsection covering the §8 18-item deferral list (17 carry-forwards + new 19.2-specific). **§13.5** HTTPFilterCallbacks AMENDMENT — add 7th NEW `BufferEncodedBody() []byte` accessor on `EncoderFilterCallbacks` (symmetric mirror of `BufferedBody()` on `DecoderFilterCallbacks`); cross-phase reuse intent for future encode-side body-transformation filters; ADR-0175 anchor reference. The 6 ADR-0174 accessors stay documented unchanged. **§13.6** Per-route canonical patterns cross-reference UNCHANGED (5th-canonical REUSE carries; ADR-0173 unchanged at 19.2). **§13.7** ext_proc framework primitive umbrella AMENDMENT — extend with NEW ADR-0175 chain-side body-buffering discipline note + cross-phase reuse intent. ~+200–300 LoC. Task 10. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `19.2` flips `in-progress → done` AT THE SAME COMMIT as parent row `19` flips `in-progress → done` per parent SPEC §8 rollup discipline. The commit-message body MUST explicitly name BOTH transitions for grep-verifiability per SPEC §15 item 12. Row `19.1` UNCHANGED (`done`). The commit message also names the 4 anchored ADRs + the D-series hypothesis dispositions. ~+2 LoC. Task 10. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state at 19.2 phase-done: `active-phase: <next-phase>` (the next ROADMAP row per parent SPEC §8 — likely the TLS-fixture phase or the next §9 family-row; consult ROADMAP at IMPL time); `lifecycle-state: phase 19.2 done; phase 19 parent done`; `next-skill: superpowers:brainstorming` (or analog — the next phase's lifecycle-state 1 session); `last-commit: <Task 10 squash SHA>`; `next-free ADR: ADR-0177` (D10 hypothesis HELD if NO impl-time-unanticipated ADR fired; otherwise next-free advances per the actual consumption — PROGRESS.md records the rationale). Task 10. |
| `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror the phase-09..19.1 PROGRESS.md structure. ~600 LoC across 11 task entries. Task 1 creates the preamble + 12-precondition verification capture; subsequent tasks each append their entry. |
| `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~280 LoC. Task 11. Verifies all 16 SPEC §15 acceptance items + all 6 phase-done gates + the 4 ADR landings + the D-series hypothesis dispositions (D10 HELD or FALSIFIED with ADR-0177 rationale) + the parent-rollup commit-message verifiability. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's 7 deferred decisions (or explicitly defer to IMPL with constraint) before implementation; this PLAN settles all 7 plus 5 PLAN-emerged decisions. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each task can act without re-deriving them. **D-series numbering picks up D14 (the 19.1 PLAN consumed D1..D14 in-PLAN + ratified D12 as the no-new-ADR hypothesis); the 19.2 PLAN starts at D1 for the 19.2-internal series** (PLAN-internal numbering; the 19.1 carry-forward hypotheses are referenced by their 19.1 names where applicable, e.g., "19.1 D12 hypothesis").

1. **D1 — `EncoderFilterCallbacks.BufferEncodedBody() []byte` method name LOCKED per SPEC §12 item 1.** The SPEC proposes `BufferEncodedBody() []byte` (mirroring `BufferedBody()` on `DecoderFilterCallbacks` for symmetric naming). PLAN LOCKS this name. Rationale: verb-first form parallels the existing `OverwriteBody()` / `AppendDecodedData()` accessor surface conventions; the verbatim mirror of `BufferedBody()` would be `BufferedEncodedBody()` — rejected because it reads less naturally at callsites (`f.ecb.BufferEncodedBody()` vs `f.ecb.BufferedEncodedBody()`). If the IMPL surfaces a strong callsite-readability argument for `BufferedEncodedBody()` instead, the rename is single-find-and-replace + the rename is documented in PROGRESS.md as a D1 disposition update. *Anchored: SPEC §3.1 + §6.5 callbacks.go row + §12 item 1.*

2. **D2 — `processFn` closure-capture layout for body-mode state LOCKED per SPEC §12 item 2.** Per ADR-0168 §Decision (xi) field-final invariant, body-mode-specific state lives in closure captures inside `processFn`. PLAN LOCKS the layout: extend the existing 19.1 `activeProcessingMode` per-direction struct with `bodyMode BodySendMode` + `bodyBuf []byte` fields (per SPEC §6.4 pseudocode). Single struct, pointer captured by `processFn`. Rationale: the 19.1 precedent already captures `*activeProcessingMode` by pointer for header-stage state; extending the same struct keeps the closure capture set unchanged. Alternative (separate `bodyState` struct per direction) rejected — adds a second pointer capture for no structural benefit. *Anchored: SPEC §6.4 + §12 item 2 + 19.1 ADR-0168 §Decision (xi).*

3. **D3 — 4-stage state-machine field consolidation: SPLIT-BY-DIRECTION per SPEC §12 item 3.** Per §6.4 + planner-time analysis: split-by-direction (decode-side `stage` enum separate from encode-side `stage` enum — both 2-valued: {headers, body, done}) rather than single 4-valued `stage` enum. Rationale: the 19.1 split-by-direction precedent reflects the parallel-dispatch reality (decode + encode dispatch independently from the framework's perspective); a single 4-valued enum would conflate the two directions and require extra synchronization at the dispatch boundary. The at-most-once-per-stage guard checks the per-direction `stage` enum against the expected stage on each callback entry; spurious entries increment `spurious_msgs_received` per the existing 19.1 discipline (UNCHANGED at 19.2). *Anchored: SPEC §6.4 + §12 item 3 + 19.1 ADR-0171 precedent.*

4. **D4 — Per-message timer behavioral enforcement = SINGLE ROLLING TIMER per direction per SPEC §12 item 4.** Per parent §5.P5 + ADR-0171 §Decision (vi): `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send. Single rolling timer per direction (NOT timer-per-stage — at-most-one-in-flight invariant per ADR-0171 §Decision (v) makes per-stage timers redundant). Cancellation propagates through the existing per-stream context-cancel discipline (`f.streamCancel()` → goroutines unblock via `ctx.Done()`). `override_message_timeout` reset path: the existing 19.1 `handleOverrideMessageTimeout` cancels the in-flight per-message timer + builds a fresh one with the override duration; the same primitive applies at body stages. The 19.1 IMPL deferred this to structural-only treatment per Carryforward O (Task 14 fix); 19.2 lifts to behavioral per SPEC §2 item 11 + SPEC §12 item 4. The timer rebuild surface is exercised by Task 8 race tests (the rebuild path against in-flight Send/Recv). *Anchored: SPEC §2 item 11 + §12 item 4 + parent §5.P5 + 19.1 Carryforward O.*

5. **D5 — Body-stage attribute envelope = HEADER-STAGE SUPERSET per SPEC §12 item 5.** Per SPEC §4.1 + planner-time analysis: body-stage attributes MIRROR header-stage attributes (the existing 19.1 CEL-attribute-name → accessor mapping carries) AND add body-stage-natural attributes (`request.size` / `response.size` populated accurately from `len(body)` rather than Content-Length-derived). Hypothesized exact additions: `request.size` / `response.size` only — no other body-stage-specific CEL attributes per the proto + reference Envoy v1.37.2 inspection (per the SPEC §4.1 hypothesis). The exact roster crystallizes at Task 9 fixture-harness scrape against reference Envoy v1.37.2's CEL attribute registry; if the scrape surfaces additional body-stage-only attributes (e.g., hypothetical `request.body_md5`), the IMPL adds them to the roster + the §6.6 hypothesis-table extension lands in `attributes.go` + PROGRESS.md documents the addition. PARSE-REJECT-time validation: an unknown CEL-attribute name in `request_attributes`/`response_attributes` continues SILENT-IGNORE per the 19.1 D3 settle (NOT PARSE-REJECT — proto-faithful with reference Envoy). *Anchored: SPEC §4.1 + §12 item 5 + 19.1 D3.*

6. **D6 — PARSE-REJECT error text for `streamed_response` LOCKED per SPEC §12 item 6.** Per §4.2 + planner-time analysis: the IMPL emits `"ext_proc: streamed_response body mutation not supported (STREAMED body modes out-of-envelope per parent §4.4)"` as the `spurious_msgs_received` increment reason + the `streams_failed` log line text. Rationale: explicit cross-reference to parent §4.4 + naming the rejected arm (`streamed_response`) at the front of the message for grep-archaeology. The classification follows ADR-0172 §Decision (iv) discipline — treated as a malformed response. *Anchored: SPEC §4.2 + §12 item 6.*

7. **D7 — Body-buffer release ordering on OnDestroy during parked body-stage outbound LOCKED per SPEC §12 item 7.** Per ADR-0169 §Decision OnDestroy discipline (REUSED unchanged at 19.2): when `OnDestroy` fires during a parked body-stage outbound, the per-stream context cancels (`f.streamCancel()`) → the body-stage Send/Recv goroutine (decode or encode side) unblocks via `ctx.Done()` → the goroutine returns WITHOUT calling `ContinueDecoding()` (decode side) or `ContinueEncoding()` (encode side). The chain dispatch tears down WITHOUT the buffer-release call firing; the per-direction `bodyBuf` is reclaimed by GC when the per-stream state struct goes out of scope. The existing 19.1 D9 race-guard primitive (`f.mu` + `f.done` per ADR-0171 §Decision) is the prerequisite (REUSED unchanged); no new race-guard primitive needed at 19.2. The race-test at Task 8 exercises the OnDestroy-during-body-stage-outbound race surface. **Chain-side `encodeBuf` discipline supplement** (PLAN-emerged): the ADR-0175 chain primitive's overflow handling MIRRORS the decode-side `errDecodeBufferOverflow` symmetrically — when the accumulated encode body exceeds `filterBufferLimitBytes`, the chain emits `errEncodeBufferOverflow` + connection reset (per the existing decode-side discipline). *Anchored: SPEC §12 item 7 + 19.1 D9 + ADR-0128 decode-side precedent.*

8. **D8 (PLAN-emerged) — 4 ADR Lands-in-Task assignments LOCKED per SPEC §10.** ADR-0175 §Decision + §Consequences at Task 2 (single-Task ADR landing per the 19.1 Task 4/Task 5 precedent for ADR-0169/ADR-0174; the chain primitive + interface extension + tests co-locate cleanly). ADR-0168 §Decision AMENDMENT at Task 3 (PARSE-REJECT lift in `extproc.go`); §Consequences refresh at Task 7 (integration completeness) per the 19.1 ADR-0168 §Decision pattern (Task 2 + Task 11). ADR-0171 §Decision AMENDMENT + §Consequences at Task 4 (single-Task — the state-machine extension + per-message timer behavioral enforcement co-locate). ADR-0172 §Decision AMENDMENT + §Consequences at Task 6 (single-Task — the body-mode arms of `applyProcessingResponse` co-locate). The implementer at each task EXTENDS the existing 19.1-anchored §Decision text with the AMENDMENT (in-place edit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '§Decision AMENDMENT' docs/envoy-go/DECISIONS.md` returning at least 3 matches post-Task 6 (Tasks 3 + 4 + 6).

9. **D9 (PLAN-emerged) — BEHAVIOR_CONTRACT 7-edit bundle lands at Task 10 (single-Task closing bundle) per SPEC §13.** Single-Task bundle rather than the 19.1 split-across-Tasks pattern (19.1 split 8 edits across Tasks 13 + 14). Rationale: the 19.2 envelope is small enough that single-Task bundle is cleaner; the §13.1 ext_proc subsection AMENDMENT depends on the Task 9 fixture scrape (for the per-scenario assertion documentation) which lands first; the §13.5 `BufferEncodedBody` accessor addition has been valid since Task 2 but lands at Task 10 to keep the BEHAVIOR_CONTRACT edits in a single grep-coherent commit. The Task 10 commit message names all 7 edits + the §13.2 stat-table UNCHANGED-at-86 confirmation.

10. **D10 (PLAN-emerged) — NO new ADR numbers consumed at 19.2 IMPL: D12/D13 hypothesis from 19.1 REASSERTED.** Strong hypothesis: the 19.2 IMPL does NOT fire `ADR-0177` (next-free stays unconsumed at 19.2 phase-done). HOLDS-IF: the SPEC §10 ADR anchor map's 4 ADRs (ADR-0175 + 3 AMENDMENTS) cover the entire 19.2 IMPL envelope; the SPEC §10 forward-pointer surfaces (buffered-body-release-vs-stream-reset interaction; `body_mutation_rules` application to body bytes) both prove UNNECESSARY (the chain-side discipline at Task 2 absorbs the release race; body_mutation_rules is header-specific per the proto). FAILS-IF: an IMPL-time discovery surfaces a load-bearing framework primitive or cross-cutting decision not anticipated at SPEC. If FALSIFIED, the new ADR is `ADR-0177` + the PROGRESS.md preamble's D10 disposition flips to FALSIFIED with rationale + the next-free ADR advances to `ADR-0178` in STATE.md. *Anchored: SPEC §10 forward-pointer analysis + parent §5 closed-block invariant + 19.1 D12 precedent.*

11. **D11 (PLAN-emerged) — 25-task / 1500-LoC PLAN gate analysis ratified per SPEC §15 + ADR-0005 §Decision 4 + SKILL_ROUTING §2.** 11 anticipated tasks (well under 25); ~830–1400 LoC production code (just under 1500 — borderline but the task-count cushion absorbs LoC variance per the phase-13..18.x precedent that LoC is soft-threshold + task-count is the canonical gate). HOLDS-IF: the 11-task structure stands through IMPL execution. FAILS-IF: an IMPL-time discovery requires a new task that pushes the count > 25 OR the LoC envelope grows substantially (> 2000 LoC production). If FALSIFIED in the LoC direction only, the task-count headroom absorbs (no re-split needed); if FALSIFIED in the task-count direction, the PLAN must be re-authored with a sub-sub-phase split (highly unlikely per the SPEC envelope analysis). *Anchored: SPEC §15 + ADR-0005 §Decision 4 + SKILL_ROUTING §2.*

12. **D12 (PLAN-emerged) — Per-route 5th-canonical body-mode arm activation LOCKED per SPEC §5.** The 19.1-anchored ADR-0173 §Decision (per-route 5th-canonical REUSE) is UNCHANGED at 19.2. `ExtProcOverrides.processing_mode`'s `request_body_mode` + `response_body_mode` arms now become CONSUMED at 19.2 (paralleling the listener-level activation per SPEC §1 item 2 + §5). Per-route override semantics UNCHANGED: REPLACES the listener-level `processing_mode` field-by-field for the listener+route merge per the proto-faithful map-merge convention. Cache-on-first-use (per ADR-0173 §Consequences from 19.1) UNCHANGED — the body-stage dispatch reads the cached `compiledConfig` from filter state without re-resolving. `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` continue SILENT-IGNORED per SPEC §2 item 12. SHARED-stats UNCHANGED. NO ADR-0173 amendment fires at 19.2. *Anchored: SPEC §5 + §2 item 12 + 19.1 ADR-0173.*

---

## ADRs introduced/amended by this plan

The 19.2-landing ADRs anticipated by SPEC §10. **All 4 anchors land in-place per ADR-0044 ADR-on-impl convention**: ADR-0175 §Decision + §Consequences full bodies (§Context already at parent SPEC commit `9cc1458`); ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS as in-place edits to existing 19.1-anchored §Decision sections. **NO new ADR numbers consumed at 19.2 PLAN per D-series D10 hypothesis** — next-free `ADR-0177` stays unconsumed; if the IMPL fires an unanticipated load-bearing ADR, it is `ADR-0177` + PROGRESS.md records the D10 hypothesis as FALSIFIED.

| ADR | Subject (19.2 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0175** | `EncoderFilterCallbacks.BufferEncodedBody() []byte` framework primitive — encode-side body-buffering analogous to phase-13 ADR-0128 decode-side; chain-side `RunEncodeData` accumulation extension (mirrors `RunDecodeData`); per-encoderCB `encodeBuf []byte` field; overflow discipline mirrors decode-side `errDecodeBufferOverflow` symmetrically per planner-time D7; cross-phase-reusable for any future encode-side body-transformation filter (a hypothetical encode-side `lua` body-callback filter; an encode-side content-injection filter — named explicitly as forward-pointers for grep-archaeology). The PLAN settles `BufferEncodedBody()` as the method name per D1 (NOT `BufferedEncodedBody()` — verb-first form parallels the existing accessor surface; mirror of `BufferedBody()` rejected for callsite readability). §Decision + §Consequences full bodies land at the same Task (single-Task ADR landing per the 19.1 Task 4 ADR-0169 precedent). | Task 2 |
| **ADR-0168 §Decision AMENDMENT** | Body-mode PARSE-REJECT lift for `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` arms when service is `grpc_service` (replaces the 19.1 `"ext_proc: request_body_mode != NONE not yet supported (lands in phase 19.2)"` error with ACCEPT-AND-WIRE dispatch). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED arms continue PARSE-REJECT permanently. HTTP-service-mode body PARSE-REJECT continues unchanged (HTTP-service is headers-only per ADR-0168 §Decision; SPEC §2 item 1). `compiledConfig` struct field-final invariant preserved per ADR-0168 §Decision (xi) — NO new fields; body-mode-specific runtime state lives in closure captures inside `processFn` per planner-time D2. §Consequences refresh at Task 7 once integration is wired (per the 19.1 ADR-0168 multi-task pattern). | Task 3 (PARSE-REJECT lift); Task 7 (integration §Consequences refresh) |
| **ADR-0171 §Decision AMENDMENT** | 4-stage state-machine extension — `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline extends to 4 stages; the 5-value action enum REUSED unchanged; per-direction state-machine field consolidation = SPLIT-BY-DIRECTION per planner-time D3; `activeProcessingMode` struct extended with per-direction `bodyMode + bodyBuf` fields per planner-time D2. `mode_override` header-response-paths-only refinement carries unchanged at 19.2 (per parent §5.P1 RATIFIED-AND-REFINED — body-stage `mode_override` silently dropped, NOT counted as spurious). **Per-message timer behavioral enforcement** lifts from structural-only (19.1) to behavioral via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send per planner-time D4 (single rolling timer per direction, NOT per-stage). The 19.1 §12 deferred decision #6 closes at 19.2 IMPL per SPEC §2 item 11. §Decision AMENDMENT + §Consequences land at the same Task. | Task 4 |
| **ADR-0172 §Decision AMENDMENT** | `body_mutation` (`*BodyMutation` oneof) — `body []byte` CONSUMED (replaces buffered body bytes + Content-Length reconciliation per ADR-0128 decode-side / ADR-0175 encode-side); `clear_body bool` CONSUMED (empties buffer + Content-Length: 0); `streamed_response *StreamedBodyResponse` PARSE-REJECT per planner-time D6 (`spurious_msgs_received` increment + malformed-response classification per ADR-0172 §Decision (iv)). `status = CONTINUE_AND_REPLACE` — at header stages with body-mode BUFFERED: combined header+body replacement (header_mutation + body_mutation both apply; body-stage outbound SKIPPED — state machine emits `actContinueButStillWaiting` from header-stage dispatcher → on body accumulation completion, skip body-stage dispatch → emit `actContinue` after applying mutations); at body stages: TREATED AS CONTINUE per the proto's "ignored at body stages" wording (no counter increment); the 19.1 spurious-dispatch for header-stages-with-body-mode-NONE LIFTS to "CONSUMED as no-op for body" per SPEC §4.3 table — 19.1 §12 deferred decision #7 CLOSED at SPEC time. Body-stage `ImmediateResponse` CONSUMED — fires `SendLocalReply` from the corresponding decode/encode-side path via the existing 19.1 multi-stage `SendLocalReply` infrastructure (REUSED unchanged) with proto-faithful status/headers/body/grpc_status. `clear_route_cache` at body stages continues IGNORED per the proto's "ignored in the response direction" wording. §Decision AMENDMENT + §Consequences land at the same Task. | Task 6 |

**NO in-place ADR-0125 amendment required by phase 19.2** (5th-canonical REUSE carries from ADR-0173 unchanged — the per-route body-mode arm activation per planner-time D12 does NOT consume a new canonical; the existing 5th canonical's `ExtProcOverrides.processing_mode` field-by-field merge discipline covers).

**ADR-0044 escape-valve held in reserve per D10** — `ADR-0177` stays unconsumed at 19.2 IMPL under the strong hypothesis. If a 19.2-IMPL-unanticipated load-bearing ADR fires (most-likely surfaces per SPEC §10: buffered-body-release-vs-stream-reset interaction with the chain primitive; `body_mutation_rules` application to body bytes — both UNLIKELY per the SPEC's analysis), it is `ADR-0177` + the PROGRESS.md D10 disposition flips to FALSIFIED + STATE.md next-free advances to `ADR-0178`.

---

## Task graph (sequential vs parallelizable)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph:

- **Task 1** (PROGRESS.md + 12-precondition verification + Carryforward dispositions from 19.1 REVIEW §10) — sequential prerequisite for everything; sets up the append-only log + records the D-series resolutions.
- **Tasks 2, 3, 5** — **PARALLELIZABLE** (three independent surfaces; all depend on Task 1 only):
  - **Task 2** — ADR-0175 framework primitive (`callbacks.go` + `chain.go` + `chain_test.go`). Completely independent of the extproc package; lands the encode-side body-buffering primitive + tests. **PRE-REQUISITE for Tasks 6 + 7** (check.go body-mode arms + extproc.go integration both consume `BufferEncodedBody()`).
  - **Task 3** — ADR-0168 §Decision AMENDMENT — extproc.go body-mode PARSE-REJECT lift + body-stage dispatch sketch (compileable skeleton; factory accepts BUFFERED but body-stage dispatch is a stub that falls through to `Continue` — full dispatch wires at Task 7). **PRE-REQUISITE for Task 7** (integration).
  - **Task 5** — attributes.go body-stage envelope builder. Completely independent of the other surfaces; the body-stage builders consume the existing ADR-0170 + ADR-0174 accessors unchanged. **PRE-REQUISITE for Task 7** (integration; the dispatch wiring calls the body-stage builders).
- **Task 4** (ADR-0171 §Decision AMENDMENT — processor.go 4-stage state machine + per-message timer behavioral enforcement) — depends on **Task 3** (the `bodyMode` flag in `activeProcessingMode` is set at parse time in Task 3's `buildCompiledConfig` skeleton). Parallelizable with Tasks 2 + 5 after Task 3 completes.
- **Task 6** (ADR-0172 §Decision AMENDMENT — check.go body-mode arms of `applyProcessingResponse`) — depends on **Task 2** (consumes `BufferEncodedBody()` for the encode-side body-mutation apply path) + **Task 4** (consumes the 4-stage state-machine constants + the per-direction state pointer). Sequential after both.
- **Task 7** (extproc.go body-stage integration — wires Tasks 2-6 into a fully-functional body-mode dispatch) — depends on **Tasks 2, 3, 4, 5, 6** (consumes all prior surfaces); produces a fully-functional body-stage dispatch from `New()`.
- **Task 8** (Race tests — OnDestroy during body-stage outbound + concurrent encodeBuf access + per-message timer cancel/rebuild) — depends on **Task 7** (full body-stage integration).
- **Task 9** (Differential fixture `0023-http-ext-proc-body` + 6 scenarios + REUSE extprocgrpc helper + Task 9 fixture-harness scrape closure for D5 attribute-roster crystallization) — depends on **Task 7** (full integration). The fixture exercises end-to-end body-mode dispatch + closes the D5 attribute-roster empirical surface.
- **Task 10** (25th fuzzer `FuzzProcessingResponseMapping` + BEHAVIOR_CONTRACT 7-edit bundle per SPEC §13 + DECISIONS.md final-state alignment + STATE/ROADMAP advance + 6 phase-done gates verification) — depends on **Task 9** (the §13.3 equivalence-matrix row + §13.4 forward-pointer notes both cite Task 9 fixture artifacts).
- **Task 11** (REVIEW per `superpowers:requesting-code-review`) — depends on **everything**; produces `REVIEW.md` + closes the phase-done gate.

**Parallel-dispatch opportunity at Tasks 2 + 3 + 5** — three agents can run concurrently on independent surfaces after Task 1 completes. **Sequential bottleneck at Tasks 4 → 6 → 7** — the state machine + check dispatcher + integration are the critical path. **Tasks 8 + 9 + 10 + 11** are sequential.

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + project memory `feedback_git_worktrees.md`. Expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-19.2-http-filter-ext-proc-body-impl \
                 -b phase-19.2-http-filter-ext-proc-body-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-19.2-http-filter-ext-proc-body-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 12 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-19.2-http-filter-ext-proc-body-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the 19.2-PLAN.md squash commit + its SHA-fill follow-up at the head, with the 19.2-SPEC.md squash commit `954a570` + its SHA-fill follow-up `b9d2b78` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `176` (ADR-0176). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR-0175 §Context present + §Decision body absent (lands at Task 2).** `grep -cE '^## ADR-0175' docs/envoy-go/DECISIONS.md` returns `1`. `grep -A20 '^## ADR-0175' docs/envoy-go/DECISIONS.md | grep -c '^### Decision'` returns `0` (§Decision body lands at Task 2). `grep -nE '^## ADR-0177' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0177 stays unconsumed under D10 hypothesis).
6. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/SPEC.md` returns `954a570` (or descendant). If different, re-read SPEC.
7. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
8. **Pristine tree.** `git status --porcelain` returns empty.
9. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
10. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-2])'` returns every fixture 0000-0022 PASS — the 23 pre-existing fixtures are the regression baseline. Phase 19.2 adds the 24th (`0023-http-ext-proc-body` per Task 9).
11. **Pre-existing fuzzers run clean at 30s.** The 24 fuzzers from phases 02-19.1 run clean. Phase 19.2 adds the 25th (`FuzzProcessingResponseMapping` per Task 10).
12. **Pre-existing 19.1-landing files present.** `test -f internal/filter/http/extproc/extproc.go && test -f internal/filter/http/extproc/processor.go && test -f internal/filter/http/extproc/check.go && test -f internal/filter/http/extproc/attributes.go && test -f internal/grpcclient/processor_client.go && test -d test/helpers/extprocgrpc && echo "ok: 19.1 surfaces present"`. The 19.2 IMPL extends these files in-place + reuses `test/helpers/extprocgrpc/` UNMODIFIED.

If all 12 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble + 19.1 Carryforward inheritance

**Files:**
- Create: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. The PROGRESS preamble ANTICIPATES the 4 ADR landings (each with its Lands-in-Task anchor reproduced from this PLAN's per-ADR table) and records the 12 planner-time decisions (D1..D12). Includes the Carryforward inheritance section explicitly listing the 19.1 REVIEW §10 forward-pointers picked up at 19.2 vs deferred further.

**Precondition:** worktree exists at `phase-19.2-http-filter-ext-proc-body-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 12 preconditions report green.
**Artifact:** `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (new file).
**Acceptance:** all 12 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` with: (a) Preamble summarizing the 12-precondition verification (verbatim command outputs captured); (b) the 4-ADR table from `## ADRs introduced/amended by this plan` reproduced verbatim; (c) the 12 planner-time decisions (D1..D12) reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Carryforward inheritance section explicitly listing the 19.1 REVIEW §10 forward-pointers picked up at 19.2 (the 17-item carry-forward + 5 new 19.2-specific) vs deferred further (Carryforward M reassignment, Carryforward R deferral, the 3 ADR-0170 envelope-content divergences, the 8 reference-Envoy counter activation, per-scenario counter-delta strict equivalence); (e) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 1: PROGRESS.md preamble + 12-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: `internal/filter/http/{callbacks,chain,chain_test}.go` — ADR-0175 §Decision + §Consequences (encode-side body-buffering framework primitive)

**Files:**
- Modify: `internal/filter/http/callbacks.go` (~+30–50 LoC; ONE new method on `EncoderFilterCallbacks`)
- Modify: `internal/filter/http/chain.go` (~+150–300 LoC; per-encoderCB `encodeBuf []byte` field + `RunEncodeData` accumulation extension + `BufferEncodedBody()` reader method on `*encoderCB`)
- Modify: `internal/filter/http/chain_test.go` (~+150–250 LoC; Group N+4 body-buffering accumulation tests + Group N+5 concurrent-access race test)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150–250 LoC: ADR-0175 §Decision + §Consequences full bodies)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 2 entry)

This task lands the NEW `EncoderFilterCallbacks.BufferEncodedBody() []byte` framework primitive per ADR-0175 — envoy-go's FIRST encode-side body-buffering primitive, symmetric mirror of phase-13 ADR-0128 decode-side `BufferedBody`. The chain-side `RunEncodeData` accumulation discipline mirrors `RunDecodeData`'s existing ADR-0128 accumulation; the per-encoderCB `encodeBuf []byte` field accumulates bytes on `DataStopIterationAndBuffer`; `ContinueEncoding()` releases the (possibly-mutated) buffer downstream + clears `encodeBuf`. Overflow handling: SYMMETRIC to decode-side `errDecodeBufferOverflow` per planner-time D7. The existing 19.1-anchored 6 ADR-0174 accessors on `EncoderFilterCallbacks` STAY UNCHANGED.

**Precondition:** Task 1 complete; ADR-0175 §Context already at parent SPEC commit `9cc1458`; chain.go:400-404 "encode-side StopIterationAndBuffer is park-only" comment present (verified at precondition 12).
**Artifact:** `internal/filter/http/{callbacks,chain}.go` with body-buffering primitive landed; `chain_test.go` with new test groups; `docs/envoy-go/DECISIONS.md` with ADR-0175 §Decision + §Consequences full bodies; `go build ./...` clean; `go vet ./...` clean; `go test -race ./internal/filter/http/...` clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/...` clean; `go test -race -count=1 -run 'TestEncoderCB_(BufferEncodedBody|ContinueEncoding|RunEncodeData)' ./internal/filter/http/...` returns all PASS; `grep -cE 'BufferEncodedBody' internal/filter/http/callbacks.go` returns ≥ 1 (the method declaration); `grep -cE '^## ADR-0175' docs/envoy-go/DECISIONS.md` returns `1` AND `grep -A5 '^### Decision' docs/envoy-go/DECISIONS.md | grep -A4 'ADR-0175' | grep -c 'BufferEncodedBody'` ≥ 1.

- [ ] **Step 1 (TDD): Author the failing test group N+4 + N+5 in `chain_test.go`** — `TestEncoderCB_BufferEncodedBody_AccumulatesAcrossMultipleEncodeData` + `TestEncoderCB_ContinueEncoding_ReleasesAccumulatedBufferAndClears` + `TestEncoderCB_RunEncodeData_OverflowEmitsErrEncodeBufferOverflow` + `TestEncoderCB_BufferEncodedBody_RaceDetectorCleanUnderConcurrentEncodeDataAndContinueEncoding`.

- [ ] **Step 2: Run the new tests to verify they FAIL** — `go test -count=1 -run 'TestEncoderCB_BufferEncodedBody' ./internal/filter/http/...` reports compilation failure (`BufferEncodedBody` method not defined) — confirms the failing-first discipline.

- [ ] **Step 3: Author `BufferEncodedBody() []byte` on `EncoderFilterCallbacks`** in `internal/filter/http/callbacks.go`. Doc-comment cites ADR-0175 + the cross-phase reuse intent (named explicitly: a hypothetical encode-side `lua` body-callback filter; an encode-side content-injection filter).

- [ ] **Step 4: Extend `*encoderCB` with `BufferEncodedBody()` reader** in `internal/filter/http/chain.go` returning the accumulated `encodeBuf` slice.

- [ ] **Step 5: Add `encodeBuf []byte` field to the per-encoderCB struct** in `chain.go`; extend `RunEncodeData` per ADR-0175 §Decision (accumulate on `DataStopIterationAndBuffer`; do NOT forward downstream; release + clear on `ContinueEncoding()`; close window at end_stream; emit `errEncodeBufferOverflow` per D7 SYMMETRIC discipline on cap exceed). The existing `encodeBufLen` running total stays as the size cap.

- [ ] **Step 6: Run the new tests to verify they PASS** — `go test -race -count=1 -run 'TestEncoderCB_(BufferEncodedBody|ContinueEncoding|RunEncodeData)' ./internal/filter/http/...` reports all PASS.

- [ ] **Step 7: Author ADR-0175 §Decision + §Consequences bodies in DECISIONS.md** — EXTENDS the existing §Context draft (at parent SPEC commit `9cc1458`) with the full §Decision body covering: the `BufferEncodedBody() []byte` method name + signature (per planner-time D1); chain-side accumulation discipline mirroring ADR-0128; `encodeBuf []byte` field on `*encoderCB`; overflow discipline mirroring decode-side `errDecodeBufferOverflow` symmetrically per planner-time D7; the SYMMETRIC pattern relative to ADR-0128; the DISTINCT semantics relative to ADR-0131 `OverwriteBody` (per-call replacement vs buffer-and-hold); cross-phase reuse intent for future encode-side body-transformation filters. §Consequences body covers: ADR-0175 is the encode-side body-handling primitive reference for ALL future §9 filters needing encode-side full-body inspection or mutation; the chain-side accumulation primitive is REUSABLE without re-deriving the discipline; the OverwriteBody (ADR-0131) primitive STAYS for per-call replacement use-cases (the two are COMPLEMENTARY).

- [ ] **Step 8: Append PROGRESS.md Task 2 entry** — record the build/vet/test output + the ADR-0175 §Decision body hash diff + `git log -1 --format=%H` for the upcoming Task 2 commit.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/callbacks.go \
        internal/filter/http/chain.go \
        internal/filter/http/chain_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 2: ADR-0175 encode-side body-buffering primitive

Lands BufferEncodedBody() on EncoderFilterCallbacks (symmetric mirror of
DecoderFilterCallbacks.BufferedBody per ADR-0175). Chain-side: per-encoderCB
encodeBuf accumulation in RunEncodeData mirroring RunDecodeData (ADR-0128).
Overflow SYMMETRIC to decode-side errDecodeBufferOverflow per planner-time D7.
ADR-0175 §Decision + §Consequences full bodies anchored (§Context already
at parent SPEC commit 9cc1458)."
```

---

## Task 3: `internal/filter/http/extproc/extproc.go` — ADR-0168 §Decision AMENDMENT (body-mode PARSE-REJECT lift for gRPC-service-mode BUFFERED)

**Files:**
- Modify: `internal/filter/http/extproc/extproc.go` (~+50–100 LoC at this task; full integration completes at Task 7)
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+100–200 LoC; Group N body-mode PARSE-REJECT lift tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+50–100 LoC: ADR-0168 §Decision AMENDMENT in-place edit)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 3 entry)

This task lifts the 19.1 body-mode PARSE-REJECT guards in `buildCompiledConfig` for `BUFFERED` arms of `request_body_mode` + `response_body_mode` when the service is `grpc_service` (ADR-0168 §Decision AMENDMENT). The factory ACCEPTS-AND-WIRES BUFFERED post-Task 3; the body-stage dispatch in `DecodeData` + `EncodeData` is a SKELETON SKETCH (returns `Continue` for now if body-mode is BUFFERED — the full dispatch wires at Task 7 integration). The `compiledConfig` struct gains NO new fields per ADR-0168 §Decision (xi) field-final invariant; body-mode-specific runtime state preparation (the per-direction `bodyMode` flag on `activeProcessingMode`) is sketched per planner-time D2 — extends the existing struct in place. STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT permanently. HTTP-service-mode body PARSE-REJECT continues unchanged.

**Precondition:** Task 1 complete; Task 2 may or may not be complete (parallelizable). The PARSE-REJECT lift is purely textual + the sketch dispatch does NOT consume ADR-0175 yet.
**Artifact:** `extproc.go` with body-mode PARSE-REJECT lifted for gRPC-service-mode BUFFERED; `extproc_test.go` with Group N tests passing; `docs/envoy-go/DECISIONS.md` with ADR-0168 §Decision AMENDMENT in-place; `go build ./...` clean; `go vet ./...` clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; `go test -count=1 -run 'TestBuildCompiledConfig_BodyMode' ./internal/filter/http/extproc/...` returns all PASS (the BUFFERED arm tests PASS + the STREAMED/PARTIAL/FULL_DUPLEX_STREAMED arms continue PARSE-REJECT); `grep -nE 'request_body_mode != NONE not yet supported' internal/filter/http/extproc/` returns 0 (the 19.1 PARSE-REJECT text removed); `grep -A3 '^## ADR-0168' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'` ≥ 1.

- [ ] **Step 1 (TDD): Author the failing tests** — Group N body-mode PARSE-REJECT lift cases in `extproc_test.go`: `TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService` + `TestBuildCompiledConfig_BodyMode_STREAMED_PARSE_REJECT_Permanent` + `TestBuildCompiledConfig_BodyMode_HTTPService_PARSE_REJECT_Continues` + `TestBuildCompiledConfig_BodyMode_BUFFERED_PerRoute_AcceptsForGRPCService`.

- [ ] **Step 2: Run the new tests to verify they FAIL** — `go test -count=1 -run 'TestBuildCompiledConfig_BodyMode_BUFFERED_AcceptsForGRPCService' ./internal/filter/http/extproc/...` returns FAIL ("PARSE-REJECT received for BUFFERED gRPC-service-mode but expected ACCEPT").

- [ ] **Step 3: Lift the body-mode PARSE-REJECT guards** in `extproc.go`'s `buildCompiledConfig` per SPEC §1 item 2 + §6.5. Add a discriminator: PARSE-REJECT continues for STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED + (HTTP-service-mode + any body mode); ACCEPTS for (gRPC-service-mode + BUFFERED). The `activeProcessingMode` per-direction struct extends with `bodyMode BodySendMode` field (sketched; the full state-machine wiring lands at Task 4).

- [ ] **Step 4: Sketch the body-stage dispatch path** in `DecodeData` + `EncodeData` — for now, return `Continue` if body-mode is BUFFERED (the full accumulate + dispatch wires at Task 7). Add a TODO comment citing Task 7 + the Task 2 / Task 4 / Task 5 / Task 6 dependencies.

- [ ] **Step 5: Run the new tests to verify they PASS** — `go test -count=1 -run 'TestBuildCompiledConfig_BodyMode' ./internal/filter/http/extproc/...` returns all PASS.

- [ ] **Step 6: Author ADR-0168 §Decision AMENDMENT in DECISIONS.md** — IN-PLACE EDIT to the existing 19.1-anchored ADR-0168 §Decision section adding the AMENDMENT clause: `**§Decision AMENDMENT (phase 19.2):** Body-mode arms (`request_body_mode = BUFFERED`, `response_body_mode = BUFFERED`) ACCEPT-AND-WIRE for `grpc_service` mode (lifts the 19.1 PARSE-REJECT). STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT permanently per parent §4.4. HTTP-service-mode body PARSE-REJECT continues unchanged (HTTP-service is headers-only per the proto's `ExtProcHttpService` constraint). The `compiledConfig` struct field-final invariant per §Decision (xi) is PRESERVED — body-mode-specific runtime state lives in closure captures inside `processFn` per planner-time D2.`

- [ ] **Step 7: Append PROGRESS.md Task 3 entry** + ADR-0168 §Decision AMENDMENT hash diff + the build/vet/test output.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/extproc/extproc.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 3: ADR-0168 §Decision AMENDMENT — body-mode PARSE-REJECT lift

Lifts the 19.1 body-mode PARSE-REJECT for gRPC-service-mode BUFFERED arms in
buildCompiledConfig. STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED
continue PARSE-REJECT permanently per parent §4.4. HTTP-service-mode body
PARSE-REJECT continues. compiledConfig field-final per §Decision (xi)
preserved — body-mode state in processFn closure captures per D2.
Body-stage dispatch sketch wires at Task 7 integration."
```

---

## Task 4: `internal/filter/http/extproc/processor.go` — ADR-0171 §Decision AMENDMENT (4-stage state machine + per-message timer behavioral enforcement)

**Files:**
- Modify: `internal/filter/http/extproc/processor.go` (~+200–350 LoC)
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+150–250 LoC; Group N+2 4-stage state-machine + Group N+6 per-message timer tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80–150 LoC: ADR-0171 §Decision AMENDMENT + §Consequences in-place edit)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 4 entry)

This task extends the per-direction `ProcessingMode` state machine from 2 stages to 4 (ADR-0171 §Decision AMENDMENT — `numStages` 2 → 4; `stageRequestBody` + `stageResponseBody` added; at-most-once-per-stage discipline EXTENDS unchanged; per-direction state SPLIT-BY-DIRECTION per planner-time D3). Extends `activeProcessingMode` struct with per-direction `bodyMode BodySendMode` + `bodyBuf []byte` fields per planner-time D2. Lifts the per-message timer from structural-only (19.1) to behavioral via single rolling timer per direction (NOT per-stage — at-most-one-in-flight invariant per ADR-0171 §Decision (v)) using `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild per planner-time D4. The `override_message_timeout` reset path extends naturally (cancels in-flight per-message timer + builds fresh one with override duration). `OnDestroy` cancels `f.streamCtx` → both decode + encode body-stage goroutines unblock via `ctx.Done()` → return WITHOUT `ContinueDecoding`/`ContinueEncoding` per planner-time D7.

**Precondition:** Task 3 complete (the `bodyMode` flag on `activeProcessingMode` exists in skeleton form).
**Artifact:** `processor.go` with 4-stage state machine + per-message timer behavioral enforcement; `extproc_test.go` with Group N+2 + Group N+6 tests passing; `docs/envoy-go/DECISIONS.md` with ADR-0171 §Decision AMENDMENT + §Consequences in-place; `go build ./...` clean; `go vet ./...` clean; `go test -race ./internal/filter/http/extproc/...` clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; `go test -race -count=1 -run 'Test(StateMachine_FourStage|PerMessageTimer_Behavioral|ModeOverride_BodyStageIgnored)' ./internal/filter/http/extproc/...` returns all PASS; `grep -cE 'stageRequestBody|stageResponseBody' internal/filter/http/extproc/processor.go` returns ≥ 2; `grep -A3 '^## ADR-0171' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'` ≥ 1.

- [ ] **Step 1 (TDD): Author failing tests** — Group N+2: `TestStateMachine_FourStage_AtMostOncePerStage` + `TestStateMachine_DecodeStageTransitions_HeadersToBodyToDone` + `TestStateMachine_EncodeStageTransitions_HeadersToBodyToDone` + `TestStateMachine_SpuriousBodyStageEntry_IncrementsSpuriousMsgsReceived`. Group N+6: `TestPerMessageTimer_Behavioral_SingleRollingTimerPerDirection` + `TestPerMessageTimer_ContextWithTimeout_CancelAndRebuildOnEachStageSend` + `TestPerMessageTimer_OverrideMessageTimeoutResetsInFlight` + `TestModeOverride_BodyStageResponse_SilentlyIgnoredNotSpurious`.

- [ ] **Step 2: Run the new tests to verify they FAIL** — `go test -count=1 -run 'Test(StateMachine_FourStage|PerMessageTimer)' ./internal/filter/http/extproc/...` returns FAIL.

- [ ] **Step 3: Extend `stage` enum** in `processor.go` adding `stageRequestBody` + `stageResponseBody` constants (per-direction; the existing 19.1 `stageRequestHeaders` + `stageResponseHeaders` carry forward; the existing `stageDone` carries forward). The 5-value action enum REUSED unchanged.

- [ ] **Step 4: Extend `activeProcessingMode` struct** with per-direction `bodyMode BodySendMode` + `bodyBuf []byte` fields per D2 + D3. The split-by-direction layout: `decode struct { stage stageEnum; bodyMode BodySendMode; bodyBuf []byte }` + `encode struct { ... }`. Initialized at parse time from `compiledConfig.processingMode`.

- [ ] **Step 5: Extend `dispatchStage` + `completeStage` discipline** to cover the 4 stages. The at-most-once-per-stage guard checks the per-direction `stage` enum on each callback entry. Spurious entries increment `spurious_msgs_received` per the existing 19.1 discipline. `mode_override` on body-stage responses silently IGNORED per parent §5.P1 RATIFIED-AND-REFINED (NOT counted as spurious).

- [ ] **Step 6: Lift the per-message timer to behavioral** per planner-time D4. Replace the 19.1 structural-only treatment with `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` cancel-and-rebuild on each stage's Send. Single rolling timer per direction. The existing `handleOverrideMessageTimeout` cancels in-flight + rebuilds.

- [ ] **Step 7: Run the new tests to verify they PASS** — `go test -race -count=1 -run 'Test(StateMachine_FourStage|PerMessageTimer|ModeOverride_BodyStage)' ./internal/filter/http/extproc/...` reports all PASS.

- [ ] **Step 8: Author ADR-0171 §Decision AMENDMENT + §Consequences in DECISIONS.md** — IN-PLACE EDIT adding the AMENDMENT clause covering: numStages 2 → 4; stageRequestBody + stageResponseBody added; at-most-once-per-stage extends; per-direction state SPLIT-BY-DIRECTION per D3; activeProcessingMode extended with bodyMode + bodyBuf per D2; mode_override header-response-paths-only refinement carries unchanged; per-message timer behavioral enforcement lifts via context.WithTimeout cancel-and-rebuild per D4 + parent §5.P5; 19.1 §12 deferred decision #6 CLOSES at this AMENDMENT.

- [ ] **Step 9: Append PROGRESS.md Task 4 entry** + ADR-0171 §Decision AMENDMENT hash diff + race-test output.

- [ ] **Step 10: Commit**

```bash
git add internal/filter/http/extproc/processor.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 4: ADR-0171 §Decision AMENDMENT — 4-stage state machine + per-message timer behavioral

Extends ProcessingMode state machine numStages 2 → 4 (stageRequestBody +
stageResponseBody added). activeProcessingMode extended with per-direction
bodyMode + bodyBuf per D2 (split-by-direction per D3). At-most-once-per-stage
extends unchanged. mode_override header-response-paths-only refinement carries
unchanged (body-stage responses silently ignored, not spurious). Per-message
timer lifts to behavioral via context.WithTimeout cancel-and-rebuild per D4
(single rolling timer per direction). 19.1 §12 deferred decision #6 closes."
```

---

## Task 5: `internal/filter/http/extproc/attributes.go` — body-stage envelope builders (consumes existing ADR-0170 + ADR-0174 accessors)

**Files:**
- Modify: `internal/filter/http/extproc/attributes.go` (~+150–250 LoC; `buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest`)
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+100–150 LoC; Group N+3 body-stage attribute envelope tests)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 5 entry)

This task adds the body-stage envelope builders. `buildRequestBodyProcessingRequest(f *filter, body []byte, endStream bool, attributeAllowlist []string) *extprocv3.ProcessingRequest` populates `ProcessingRequest.request_body = &HttpBody{body: body, end_of_stream: endStream}` + `ProcessingRequest.attributes` map per planner-time D5 (header-stage SUPERSET — mirrors header-stage CEL-attribute-name → accessor mapping + adds body-stage-natural attributes `request.size` populated from `len(body)`). Symmetric `buildResponseBodyProcessingRequest`. The 19.1-landing header-stage builders STAY UNCHANGED. The existing helpers (`lowercaseHeaderMap`, `sourcePrincipalFirstOrEmpty`) are REUSED unchanged. The exact body-stage attribute roster crystallizes empirically at Task 9 fixture-harness scrape; this task lands the PLAN-time hypothesis (header-stage roster + `request.size`/`response.size`).

**Precondition:** Task 1 complete. Independent of Tasks 2/3/4 (does not consume ADR-0175 chain primitive or the 4-stage state-machine constants directly; the builders are pure functions over body + headers + the chain-field accessors).
**Artifact:** `attributes.go` with body-stage builders; `extproc_test.go` with Group N+3 tests passing; `go build ./...` clean; `go vet ./...` clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; `go test -count=1 -run 'TestBuildBodyProcessingRequest' ./internal/filter/http/extproc/...` returns all PASS; `grep -cE 'buildRequestBodyProcessingRequest|buildResponseBodyProcessingRequest' internal/filter/http/extproc/attributes.go` returns ≥ 2.

- [ ] **Step 1 (TDD): Author failing tests** — Group N+3: `TestBuildBodyProcessingRequest_PopulatesRequestBodyField` + `TestBuildBodyProcessingRequest_AttributesEnvelopeMirrorsHeaderStage` + `TestBuildBodyProcessingRequest_RequestSizePopulatesFromBodyLength` + `TestBuildResponseBodyProcessingRequest_Symmetric`.

- [ ] **Step 2: Run the new tests to verify they FAIL** — `go test -count=1 -run 'TestBuildBodyProcessingRequest' ./internal/filter/http/extproc/...` returns FAIL ("buildRequestBodyProcessingRequest not defined").

- [ ] **Step 3: Author `buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest`** in `attributes.go` per the row in `## File structure` above + planner-time D5 attribute roster.

- [ ] **Step 4: Run the new tests to verify they PASS** — `go test -count=1 -run 'TestBuildBodyProcessingRequest' ./internal/filter/http/extproc/...` reports all PASS.

- [ ] **Step 5: Append PROGRESS.md Task 5 entry** + the build/test output.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extproc/attributes.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 5: body-stage attribute envelope builders

Lands buildRequestBodyProcessingRequest + buildResponseBodyProcessingRequest
in attributes.go. Body-stage attribute roster mirrors header-stage per D5
(SUPERSET; adds request.size/response.size populated from len(body)).
Exact roster crystallizes at Task 9 fixture-harness scrape."
```

---

## Task 6: `internal/filter/http/extproc/check.go` — ADR-0172 §Decision AMENDMENT (body-mode arms of `applyProcessingResponse`)

**Files:**
- Modify: `internal/filter/http/extproc/check.go` (~+150–250 LoC)
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+200–300 LoC; Group N+1 body-stage applyProcessingResponse tests)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80–150 LoC: ADR-0172 §Decision AMENDMENT + §Consequences in-place edit)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 6 entry)

This task activates the body-mode arms of `applyProcessingResponse` (ADR-0172 §Decision AMENDMENT). The 7-step dispatcher itself REUSED unchanged from 19.1; 19.2 ACTIVATES three previously-spurious arms per SPEC §4.2 + §4.3 + §4.4: (a) `body_mutation` switch (`body` + `clear_body` CONSUMED; `streamed_response` PARSE-REJECT per D6); (b) `CONTINUE_AND_REPLACE` at header stages with body-mode BUFFERED → combined header+body replacement + body-stage outbound SKIPPED; at body stages → TREATED AS CONTINUE; (c) body-stage `ImmediateResponse` fires `SendLocalReply` via the existing multi-stage infrastructure. Content-Length reconciliation per ADR-0128 (decode side) + ADR-0175 (encode side) when body length changes. `clear_route_cache` at body stages continues IGNORED.

**Precondition:** Task 2 complete (consumes `BufferEncodedBody()` for the encode-side body-mutation apply); Task 4 complete (consumes the 4-stage state-machine constants + per-direction state pointer).
**Artifact:** `check.go` with body-mode arms; `extproc_test.go` with Group N+1 tests passing; `docs/envoy-go/DECISIONS.md` with ADR-0172 §Decision AMENDMENT + §Consequences in-place; race-tests clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; `go test -race -count=1 -run 'TestApplyProcessingResponse_(BodyMutation|ContinueAndReplace|BodyStageImmediateResponse)' ./internal/filter/http/extproc/...` returns all PASS; `grep -A3 '^## ADR-0172' docs/envoy-go/DECISIONS.md | grep -c '§Decision AMENDMENT'` ≥ 1.

- [ ] **Step 1 (TDD): Author failing tests** — Group N+1: `TestApplyProcessingResponse_BodyMutation_Body_ReplacesBufferAndReconcilesContentLength` + `TestApplyProcessingResponse_BodyMutation_ClearBody_EmptiesBuffer` + `TestApplyProcessingResponse_BodyMutation_StreamedResponse_PARSE_REJECT_SpuriousMsgsReceivedIncrement` + `TestApplyProcessingResponse_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_CombinedReplacement_BodyStageOutboundSKIPPED` + `TestApplyProcessingResponse_ContinueAndReplace_BodyStage_TreatedAsContinue_NoCounterIncrement` + `TestApplyProcessingResponse_BodyStageImmediateResponse_FiresSendLocalReply` + `TestApplyProcessingResponse_ClearRouteCacheAtBodyStage_Ignored`.

- [ ] **Step 2: Run the new tests to verify they FAIL** — `go test -count=1 -run 'TestApplyProcessingResponse_BodyMutation' ./internal/filter/http/extproc/...` returns FAIL.

- [ ] **Step 3: Add body_mutation switch arms** in `applyProcessingResponse` per SPEC §4.2 + planner-time D6 error text for streamed_response.

- [ ] **Step 4: Add CONTINUE_AND_REPLACE handling** per SPEC §4.3 — combined replacement at header stages with body-mode BUFFERED; TREATED AS CONTINUE at body stages; the 19.1 spurious-dispatch for header-stages-with-body-mode-NONE LIFTS to "CONSUMED as no-op for body".

- [ ] **Step 5: Wire body-stage ImmediateResponse** via the existing 19.1 multi-stage `SendLocalReply` infrastructure. `clear_route_cache` at body stages continues IGNORED per SPEC §4.4.

- [ ] **Step 6: Run the new tests to verify they PASS** — `go test -race -count=1 -run 'TestApplyProcessingResponse_(BodyMutation|ContinueAndReplace|BodyStageImmediateResponse|ClearRouteCacheAtBodyStage)' ./internal/filter/http/extproc/...` reports all PASS.

- [ ] **Step 7: Author ADR-0172 §Decision AMENDMENT + §Consequences in DECISIONS.md** — IN-PLACE EDIT adding the AMENDMENT clause covering: body_mutation body/clear_body CONSUMED + streamed_response PARSE-REJECT per D6; CONTINUE_AND_REPLACE combined replacement at header stages with body-mode BUFFERED + TREATED AS CONTINUE at body stages; 19.1 spurious-dispatch lifts; body-stage ImmediateResponse CONSUMED via existing multi-stage SendLocalReply; clear_route_cache at body stages IGNORED. Cross-reference 19.1 §12 deferred decision #7 closure at SPEC time per SPEC §4.3 table.

- [ ] **Step 8: Append PROGRESS.md Task 6 entry** + ADR-0172 §Decision AMENDMENT hash diff + race-test output.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/extproc/check.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 6: ADR-0172 §Decision AMENDMENT — body-mode applyProcessingResponse arms

Activates body_mutation (body/clear_body CONSUMED; streamed_response
PARSE-REJECT per D6 with spurious_msgs_received increment).
CONTINUE_AND_REPLACE combined replacement at header stages with body-mode
BUFFERED + TREATED AS CONTINUE at body stages (19.1 spurious-dispatch lifts).
Body-stage ImmediateResponse fires SendLocalReply via existing multi-stage
infrastructure. clear_route_cache at body stages IGNORED. 19.1 §12 deferred
decision #7 closure at SPEC time cross-referenced."
```

---

## Task 7: `internal/filter/http/extproc/extproc.go` — body-stage integration (wires Tasks 2-6 into full body-mode dispatch) + ADR-0168 §Consequences refresh

**Files:**
- Modify: `internal/filter/http/extproc/extproc.go` (~+100–200 LoC; `DecodeData`/`EncodeData` full bodies; `processFn` closure-capture envelope completion)
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+150–250 LoC; integration tests + per-message timer + OnDestroy)
- Modify: `docs/envoy-go/DECISIONS.md` (~+30–60 LoC: ADR-0168 §Consequences refresh)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 7 entry)

This task wires Tasks 2-6 into a fully-functional body-mode dispatch. Completes the `DecodeData` + `EncodeData` body-stage dispatch per SPEC §6.3 pseudocode: when `cfg.bodyModeActive(direction) && !endStream` → accumulate via ADR-0128 (decode) or ADR-0175 (encode); when `endStream` → emit `ProcessingRequest{request_body|response_body}` via Task 5 attribute builders + park goroutine on resume channel; on RECV apply Task 6's body-mode arms; on resume call `ContinueDecoding`/`ContinueEncoding`. Completes the `processFn` closure-capture envelope per planner-time D2 (extends per-direction state pointer + body-buffer access). Refreshes ADR-0168 §Consequences per the 19.1 ADR-0168 multi-task pattern (Task 2 lands §Decision draft; Task 11 lands §Consequences once integration is wired).

**Precondition:** Tasks 2, 3, 4, 5, 6 ALL complete.
**Artifact:** `extproc.go` with full body-mode dispatch; `extproc_test.go` with integration tests passing end-to-end; `docs/envoy-go/DECISIONS.md` with ADR-0168 §Consequences refreshed; race-tests clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; `go test -race -count=1 ./internal/filter/http/extproc/...` returns all PASS; `go test -race -count=1 ./...` returns clean repo-wide; `grep -A20 '^## ADR-0168' docs/envoy-go/DECISIONS.md | grep -c '§Consequences'` ≥ 1 (the §Consequences refresh present).

- [ ] **Step 1 (TDD): Author failing integration tests** — `TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation` + `TestExtProc_ResponseBodyBuffered_EndToEnd_WithMutation` + `TestExtProc_BodyStageImmediateResponse_EndToEnd` + `TestExtProc_ContinueAndReplace_HeaderStageWithBodyModeBUFFERED_EndToEnd_BodyStageOutboundSKIPPED` + `TestExtProc_OnDestroy_DuringBodyStageOutbound_NoBufferReleaseFires`.

- [ ] **Step 2: Run the integration tests to verify they FAIL** — `go test -count=1 -run 'TestExtProc_RequestBodyBuffered_EndToEnd_WithMutation' ./internal/filter/http/extproc/...` returns FAIL.

- [ ] **Step 3: Complete `DecodeData` body-stage dispatch** in `extproc.go` per SPEC §6.3 pseudocode + planner-time D7 OnDestroy discipline.

- [ ] **Step 4: Complete `EncodeData` body-stage dispatch** symmetrically using ADR-0175 `BufferEncodedBody()`.

- [ ] **Step 5: Complete the `processFn` closure-capture envelope** per planner-time D2 — extends per-direction state pointer + body-buffer access; the body-buffer pointers (decode + encode) are captured by pointer.

- [ ] **Step 6: Run the integration tests to verify they PASS** — `go test -race -count=1 ./internal/filter/http/extproc/...` reports all PASS.

- [ ] **Step 7: Refresh ADR-0168 §Consequences in DECISIONS.md** — extend the 19.1-anchored ADR-0168 §Consequences with the 19.2 integration completeness clause: the body-mode arm activation is wired end-to-end; the `compiledConfig` struct field-final invariant is preserved (verified via grep of the struct definition); the `processFn` closure captures body-buffer pointers per D2; body-stage dispatch produces the same 5-value action enum as header-stage dispatch.

- [ ] **Step 8: Append PROGRESS.md Task 7 entry** + integration-test output + the ADR-0168 §Consequences refresh hash diff.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/extproc/extproc.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 7: extproc.go body-stage integration + ADR-0168 §Consequences refresh

Wires Tasks 2-6 into full body-mode dispatch. DecodeData accumulates via
ADR-0128 reuse + EncodeData accumulates via ADR-0175. processFn closure
captures body-buffer pointers per D2. End-to-end integration tests pass:
request_body/response_body mutation, body-stage immediate_response,
CONTINUE_AND_REPLACE combined replacement + body-stage outbound SKIPPED,
OnDestroy during body-stage outbound (no buffer-release fires per D7).
ADR-0168 §Consequences refreshed with integration completeness clause."
```

---

## Task 8: Race tests — OnDestroy during body-stage outbound + concurrent `encodeBuf` access + per-message timer cancel/rebuild

**Files:**
- Modify: `internal/filter/http/extproc/extproc_test.go` (~+150–250 LoC; race-test group)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 8 entry)

This task exercises the body-stage race surfaces under `-race`. Per SPEC §14.2: (a) OnDestroy during in-flight body-stage outbound — same primitive 19.1 ratified for header-stage outbound; the body-stage Send/Recv loop honors `ctx.Done()` and returns promptly per D7. (b) Chain-side `encodeBuf` accumulation concurrent with `ContinueEncoding()` — exercised at Task 2 already (`chain_test.go` Group N+5) but ALSO exercised end-to-end here via the body-stage dispatch path. (c) Per-message timer behavioral enforcement — the `context.WithTimeout` cancel-and-rebuild discipline introduces a new cancellation surface; the race detector exercises the rebuild path against in-flight Send/Recv. (d) `mode_override` re-eval on header-stage responses race against body-stage dispatch — confirms the body-stage dispatch reads the post-override `bodyMode` correctly.

**Precondition:** Task 7 complete.
**Artifact:** Race tests added; `go test -race ./...` clean repo-wide.
**Acceptance:** `go test -race -count=1 -run 'TestRace_(OnDestroyDuringBodyStageOutbound|EncodeBufConcurrentWithContinueEncoding|PerMessageTimerCancelRebuild|ModeOverrideVsBodyStageDispatch)' ./internal/filter/http/extproc/...` returns all PASS; `go test -race -count=1 ./...` repo-wide returns 0 FAIL.

- [ ] **Step 1: Author race tests** — `TestRace_OnDestroyDuringBodyStageOutbound_DecodeAndEncode` (parallels 19.1's header-stage equivalent); `TestRace_EncodeBufConcurrentWithContinueEncoding_EndToEnd`; `TestRace_PerMessageTimerCancelRebuild_AgainstInFlightSendRecv`; `TestRace_ModeOverrideHeaderStageResponse_VsBodyStageDispatch`.

- [ ] **Step 2: Run tests** — `go test -race -count=1 -run 'TestRace_' ./internal/filter/http/extproc/...` reports all PASS.

- [ ] **Step 3: Run race-test repo-wide** — `go test -race -count=1 ./...` reports 0 FAIL across all packages.

- [ ] **Step 4: Append PROGRESS.md Task 8 entry** + race-test output.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 8: race tests — body-stage OnDestroy + encodeBuf concurrency + per-message timer

Adds race tests for body-stage surfaces: OnDestroy during in-flight body-stage
outbound (decode + encode) per D7; encodeBuf concurrent access with
ContinueEncoding end-to-end; per-message timer context.WithTimeout cancel +
rebuild against in-flight Send/Recv; mode_override on header-stage response
vs body-stage dispatch read of bodyMode. Race-test repo-wide clean."
```

---

## Task 9: Differential fixture `0023-http-ext-proc-body` + 6 scenarios + REUSE `test/helpers/extprocgrpc/` + D5 attribute-roster crystallization

**Files:**
- Create: `test/fixtures/0023-http-ext-proc-body/{envoy.yaml,envoy-go.yaml,expectations.yaml,README.md,inputs/driver.go}` (~+1140 LoC)
- Modify: `test/differential/runner_test.go` (~+1 LoC; blank import for `0023/inputs`)
- Modify: `internal/filter/http/extproc/attributes.go` (~+0–30 LoC; D5 attribute-roster fix-ups if the fixture scrape surfaces deltas)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 9 entry)

This task lands the differential fixture per SPEC §7. Three-listener topology REUSED from 0022 (l_test_a/b/c). `test/helpers/extprocgrpc/` REUSED UNMODIFIED per SPEC §7.1. 6 scenarios per SPEC §7.2 — (a) request_body_buffered_mutation; (b) response_body_buffered_mutation; (c) body_stage_immediate_response; (d) body_stage_clear_body; (e) header_stage_continue_and_replace; (f) per_route_body_mode_override. Counter-delta PRESENCE-check per ADR-0173 §Consequences AMENDMENT (carry-forward from 19.1). The fixture-harness scrape closes planner-time D5 attribute-roster crystallization (if the scrape against reference Envoy v1.37.2 surfaces a body-stage-only CEL attribute beyond the PLAN's hypothesis, `attributes.go` adjusts).

**Precondition:** Task 8 complete; body-stage dispatch race-clean.
**Artifact:** `test/fixtures/0023-http-ext-proc-body/` directory; runner_test.go blank import; attributes.go D5 fix-ups if any; differential suite green at fixture 0023.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'TestDifferential/0023-http-ext-proc-body'` returns PASS; the 24 pre-existing fixtures (0000-0022) STAY PASS; D5 attribute-roster crystallization recorded in PROGRESS.md (HOLDS or AMENDED).

- [ ] **Step 1: Author the fixture YAMLs + driver + README + expectations** per the `## File structure` rows above. Three-listener topology REUSED from 0022; processor cluster + echobackend cluster REUSED.

- [ ] **Step 2: Author the 6 scenario driver functions** per SPEC §7.2 in `inputs/driver.go`. Scripted processor responses via `srv.Script(":path-discriminator", responses...)`.

- [ ] **Step 3: Add blank import to `runner_test.go`** — `_ "github.com/esalaine/envoy-go/test/fixtures/0023-http-ext-proc-body/inputs"`.

- [ ] **Step 4: Run the differential suite** — `go test -count=1 ./test/differential/...` reports fixture 0023 PASS + the 24 pre-existing fixtures STAY PASS.

- [ ] **Step 5: Capture the D5 attribute-roster empirical scrape** — `setupProcessorGRPC` server captures the `ProcessingRequest{request_body}` + `ProcessingRequest{response_body}` envelopes received during scenarios a + b + d; assert the CEL-attribute-name roster against the planner-time D5 hypothesis (header-stage SUPERSET; `request.size`/`response.size` populated from `len(body)`). If the scrape surfaces a divergence (unanticipated body-stage-only attribute), adjust `attributes.go` + re-run. **Any D5-driven `attributes.go` amendment lands in the Task 9 commit** (the `git add` list at Step 7 already includes `internal/filter/http/extproc/attributes.go` for exactly this disposition); PROGRESS.md Step 6 records the AMENDED roster verbatim.

- [ ] **Step 6: Append PROGRESS.md Task 9 entry** + the differential-suite output + the D5 disposition (HOLDS or AMENDED with the adjusted attribute roster).

- [ ] **Step 7: Commit**

```bash
git add test/fixtures/0023-http-ext-proc-body/ \
        test/differential/runner_test.go \
        internal/filter/http/extproc/attributes.go \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 9: differential fixture 0023-http-ext-proc-body + 6 scenarios + D5 crystallization

Lands fixture 0023 covering 6 body-stage scenarios per SPEC §7.2.
Three-listener topology REUSED from 0022; test/helpers/extprocgrpc/ REUSED
UNMODIFIED. Counter-delta PRESENCE-check per ADR-0173 §Consequences AMENDMENT
carry-forward. D5 attribute-roster crystallization HELD (or AMENDED per the
fixture scrape disposition). 24 pre-existing fixtures (0000-0022) STAY PASS."
```

---

## Task 10: 25th fuzzer `FuzzProcessingResponseMapping` + BEHAVIOR_CONTRACT 7-edit bundle (SPEC §13) + DECISIONS final-state alignment + STATE/ROADMAP advance + 6 phase-done gates

**Files:**
- Modify: `internal/filter/http/extproc/fuzz_test.go` (~+100 LoC; 25th fuzzer `FuzzProcessingResponseMapping`)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (~+200–300 LoC; 7-edit bundle per SPEC §13 + planner-time D9)
- Modify: `docs/envoy-go/DECISIONS.md` (~+0–30 LoC; final-state alignment if any §Consequences need polishing)
- Modify: `docs/envoy-go/ROADMAP.md` (~+2 LoC; row `19.2` `in-progress → done` + parent row `19` `in-progress → done` AT THE SAME COMMIT)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place per BOOTSTRAP §5)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 10 entry)

This task lands the 25th fuzzer + the BEHAVIOR_CONTRACT 7-edit bundle (planner-time D9 single-Task closing bundle) + the DECISIONS.md final-state alignment + the STATE/ROADMAP advance + the 6 phase-done gates verification per BOOTSTRAP §7.5. The commit message MUST explicitly name BOTH ROADMAP row transitions (19.2 + parent 19) for grep-verifiability per SPEC §15 item 12. The 6 phase-done gates verified at this task: Gate A (build + vet + lint); Gate B (race-tests); Gate C (h2spec 53/53 PASS at ADR-0051 pin); Gate D (25 fuzzers GREEN at 30s); Gate E (24 differential fixtures PASS — 0000-0023); Gate F (BEHAVIOR_CONTRACT bundle landed).

**Precondition:** Task 9 complete; differential fixture 0023 green; D5 disposition recorded.
**Artifact:** 25th fuzzer landed; BEHAVIOR_CONTRACT 7-edit bundle landed; DECISIONS.md final; ROADMAP row 19.2 + parent 19 BOTH flipped to done; STATE.md advanced; PROGRESS.md Task 10 entry with all 6 gates GREEN output captured.
**Acceptance:** All 6 phase-done gates GREEN at this commit; `grep -cE 'FuzzProcessingResponseMapping' internal/filter/http/extproc/fuzz_test.go` returns ≥ 1; `find . -name 'fuzz_test.go' | wc -l` returns 25 (24 pre-existing + 25th NEW); `grep -E '^\| 19\.2 .* done' docs/envoy-go/ROADMAP.md` matches AND `grep -E '^\| 19 .* done' docs/envoy-go/ROADMAP.md` matches AT THE SAME COMMIT (the squash commit per the next section).

- [ ] **Step 1 (TDD): Author 25th fuzzer `FuzzProcessingResponseMapping`** per SPEC §7.3 — corpus seeds cover body-stage `CommonResponse` + `body_mutation` + `CONTINUE_AND_REPLACE` arms; 30s ADR-0018 budget. The existing 24th fuzzer `FuzzExtProcConfigParse` STAYS UNCHANGED.

- [ ] **Step 2: Run the new fuzzer at 30s** — `go test -fuzz=FuzzProcessingResponseMapping -fuzztime=30s ./internal/filter/http/extproc/...` reports PASS (no panic; no block; spurious increments bounded).

- [ ] **Step 3: Author the 7 BEHAVIOR_CONTRACT edits** per SPEC §13 + planner-time D9 (single-Task closing bundle): §13.1 ext_proc subsection body-mode AMENDMENT; §13.2 stat-name table UNCHANGED-at-86 confirmation; §13.3 Equivalence Matrix NEW row for 0023; §13.4 NEW `### Phase 19.2 forward-pointer notes`; §13.5 HTTPFilterCallbacks AMENDMENT adding 7th NEW `BufferEncodedBody`; §13.6 Per-route canonical patterns cross-reference UNCHANGED; §13.7 ext_proc framework primitive umbrella AMENDMENT (chain-side body-buffering note per ADR-0175).

- [ ] **Step 4: Final-state alignment of DECISIONS.md** — verify all 4 ADR landings cleanly anchored: ADR-0175 §Decision + §Consequences full (Task 2); ADR-0168 §Decision AMENDMENT + §Consequences refresh (Tasks 3 + 7); ADR-0171 §Decision AMENDMENT + §Consequences (Task 4); ADR-0172 §Decision AMENDMENT + §Consequences (Task 6). Polish any prose if needed (typo fixes, cross-reference completeness).

- [ ] **Step 5: ROADMAP advance** — row `19.2` `in-progress → done` + parent row `19` `in-progress → done` AT THE SAME COMMIT per parent SPEC §8 rollup discipline. Last-touched dates updated to today.

- [ ] **Step 6: STATE.md rewrite-in-place** per BOOTSTRAP §5 — `active-phase` advances to the next ROADMAP row; `lifecycle-state: phase 19.2 done; phase 19 parent done`; `next-skill: superpowers:brainstorming` (or analog for the next-phase lifecycle-state 1 session); `last-commit: <Task 10 squash SHA — TBD before squash; SHA-fill follow-up after squash-merge>`; `next-free ADR: ADR-0177` (D10 hypothesis HELD if NO impl-time-unanticipated ADR fired); `last-updated: <impl-date>`.

- [ ] **Step 7: Verify all 6 phase-done gates** — capture verbatim outputs in PROGRESS.md Task 10 entry:
  - Gate A: `go build ./... && go vet ./...` clean.
  - Gate B: `golangci-lint run ./...` clean.
  - Gate C: h2spec 53/53 PASS at ADR-0051 pin.
  - Gate D: `find . -name 'fuzz_test.go' | wc -l` = 25; representative spot-check 5 fuzzers at 30s each all PASS (including `FuzzProcessingResponseMapping`).
  - Gate E: `go test -count=1 ./test/differential/...` reports 24/24 fixtures PASS (0000-0023).
  - Gate F: `tools/check_behavior_contract.sh` (or analog) clean; stat-name table at 86 names confirmed.

- [ ] **Step 8: Append PROGRESS.md Task 10 entry** with all 6 gate outputs verbatim + the D-series final dispositions (D1..D12 — all HELD, except D5 if amended at Task 9; D10 HELD if no ADR-0177 fired).

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/extproc/fuzz_test.go \
        docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/STATE.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 10: 25th fuzzer + BEHAVIOR_CONTRACT 7-edit bundle + DECISIONS final + STATE/ROADMAP advance

25th fuzzer FuzzProcessingResponseMapping clean at 30s. BEHAVIOR_CONTRACT
7-edit bundle landed per SPEC §13 + planner-time D9 (single-Task closing).
DECISIONS final-state aligned for 4 ADR landings (ADR-0175 §Decision +
§Consequences full bodies; ADR-0168 / ADR-0171 / ADR-0172 §Decision
AMENDMENTS in-place per ADR-0044). ROADMAP row 19.2 in-progress → done
AND parent row 19 in-progress → done AT THE SAME COMMIT per parent SPEC §8.
STATE.md advanced (next-free ADR-0177 stays unconsumed under D10 hypothesis).
All 6 phase-done gates GREEN."
```

---

## Task 11: REVIEW per `superpowers:requesting-code-review`

**Files:**
- Create: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/REVIEW.md` (~280 LoC)
- Append: `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md` (Task 11 entry)

This task runs the end-of-phase code review per `superpowers:requesting-code-review`. Mirror phase-09..19.1 REVIEW.md structure. Verifies the 16 SPEC §15 acceptance items + the 6 phase-done gates per BOOTSTRAP §7.5 + the 4 ADR landings + the 12 D-series dispositions + the parent-rollup commit-message verifiability (BOTH 19.2 + 19 transitions named in the squash commit body).

**Precondition:** Tasks 1-10 complete; all 6 phase-done gates verified GREEN at Task 10.
**Artifact:** `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/REVIEW.md` (NEW).
**Acceptance:** REVIEW.md addresses all 16 SPEC §15 acceptance items + identifies the 6 phase-done gates as GREEN + lists the 4 ADR landings + confirms ADR-0044 escape-valve disposition (D10 hypothesis HELD or FALSIFIED with rationale) + records the D-series final dispositions (D1..D12); review-document-reviewer subagent or human reviewer approves.

- [ ] **Step 1: Invoke `superpowers:requesting-code-review`** — per the skill's discipline; provide PLAN.md path + SPEC.md path + STATE.md path. The skill dispatches the reviewer with the focused context.

- [ ] **Step 2: Address reviewer feedback** — fix issues in-place (per `superpowers:receiving-code-review`); iterate until approved.

- [ ] **Step 3: Author `REVIEW.md`** — `## Phase 19.2 review` + sections for each acceptance item (1-16); each section captures: "Acceptance criterion verbatim from SPEC §15", "Evidence of satisfaction (commit hash + file path + grep verification)", "Status: GREEN / RED-with-remediation". + `## Phase-done gates` (A/B/C/D/E/F per BOOTSTRAP §7.5) each marked GREEN with evidence. + `## ADR roster` listing the 4 ADR landings + their Lands-in-Tasks + the ADR-0044 escape-valve disposition. + `## D-series dispositions` (D1..D12 HELD/AMENDED/FALSIFIED). + `## Parent-rollup verification` (BOTH ROADMAP row transitions present in squash commit body). + `## Forward-pointers carried into 19.3 / next phase` (the 17 carry-forwards + 5 new 19.2-specific deferred items + 2 new 19.2 closures of 19.1 forward-pointers).

- [ ] **Step 4: Append PROGRESS.md Task 11 entry** + record the reviewer approval signal.

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/phases/19.2-http-filter-ext-proc-body/REVIEW.md \
        docs/envoy-go/phases/19.2-http-filter-ext-proc-body/PROGRESS.md
git commit -m "phase 19.2 Task 11: REVIEW.md + reviewer approval

Phase 19.2 closing review per superpowers:requesting-code-review.
All 16 SPEC §15 acceptance items GREEN; all 6 phase-done gates GREEN;
4 ADR landings verified (ADR-0175 §Decision + §Consequences full;
ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS in-place per ADR-0044);
D-series D1..D12 dispositions captured; parent-rollup commit verifiability
confirmed (BOTH 19.2 + 19 transitions named in squash commit body).
Forward-pointers carried into 19.3 / next phase."
```

---

## Phase-done squash-merge + push to origin (closes BOTH row 19.2 AND parent row 19 AT THE SAME COMMIT)

After Task 11 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-19.2-http-filter-ext-proc-body-impl
# Resolve commit message — body must EXPLICITLY name BOTH ROADMAP row transitions
# per SPEC §15 item 12 for grep-verifiability.
git commit -m "$(cat <<'EOF'
Squash merge phase-19.2-http-filter-ext-proc-body-impl

Closes ROADMAP row 19.2 (in-progress → done) AND parent row 19
(in-progress → done) AT THE SAME COMMIT per parent SPEC §8 rollup discipline.

11 tasks landed. 4 ADRs anchored: ADR-0175 §Decision + §Consequences full
(encode-side BufferEncodedBody framework primitive — symmetric mirror of
phase-13 ADR-0128 decode-side); ADR-0168 / ADR-0171 / ADR-0172 §Decision
AMENDMENTS in-place per ADR-0044 (body-mode PARSE-REJECT lift for
gRPC-service-mode BUFFERED; 4-stage state machine + per-message timer
behavioral enforcement; body_mutation + CONTINUE_AND_REPLACE + body-stage
ImmediateResponse activation). D10 hypothesis HELD: no impl-time-unanticipated
ADR fired; next-free ADR-0177 stays unconsumed. 25th fuzzer
FuzzProcessingResponseMapping clean at 30s. 24/24 differential fixtures green
(0000-0023). All 6 phase-done gates green.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..19.1 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 10):
# Edit docs/envoy-go/STATE.md replacing "<Task 10 commit SHA — TBD before squash; SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 19.2 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-19.2-http-filter-ext-proc-body-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit.
```

---

## Remember
- Exact file paths always.
- Complete code shapes in the SPEC §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor; mirrors the 19.1 PLAN's discipline).
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 8), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit), `@superpowers:requesting-code-review` (Task 11), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 10).
- DRY, YAGNI, TDD, frequent commits.

End of phase 19.2 PLAN.
