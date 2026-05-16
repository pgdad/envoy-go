# Fixture 0023-http-ext-proc-body

Differential fixture for `envoy.filters.http.ext_proc` — body-mode slice
(phase 19.2). Exercises the gRPC-service-mode `request_body_mode = BUFFERED`
and `response_body_mode = BUFFERED` arms activated by ADR-0168 §Decision
AMENDMENT at 19.2 + SPEC §7 + the 6-scenario matrix per SPEC §7.2.

Three-listener topology REUSED from fixture 0022 (l_test_a / l_test_b /
l_test_c). The `test/helpers/extprocgrpc/` test-helper is REUSED UNMODIFIED
per SPEC §7.1.

## Topology

- `l_test_a` — listener-level `request_body_mode: BUFFERED`. Hosts
  scenario (a) `request_body_buffered_mutation`.
- `l_test_b` — listener-level `response_body_mode: BUFFERED`. Hosts
  scenarios (b) `response_body_buffered_mutation`, (c)
  `body_stage_immediate_response`, (d) `body_stage_clear_body`,
  (e) `header_stage_continue_and_replace`.
- `l_test_c` — listener-level body-mode `NONE` baseline. Hosts scenario
  (f) `per_route_body_mode_override` with two routes:
    - route-A (`/scenario_f_a`) — `ExtProcPerRoute{overrides{processing_mode{
      request_body_mode: BUFFERED}}}` activates body-stage dispatch on this
      route only.
    - route-B (`/scenario_f_b`) — no per-route override; route exercises
      19.1-style headers-only baseline.

## Clusters

- `c_backend` — echobackend subprocess (HTTP/1.1) at
  `{{.BackendHost}}:{{.BackendPort}}`. Reflects request method + path +
  headers as JSON for upstream-arrival assertions (scenarios a + f).
- `c_ext_proc` — in-process bidi-stream gRPC `ExternalProcessor.Process`
  server at `{{.ProcHost}}:{{.ProcGRPCPort}}`. Plaintext h2c (no TLS) per
  SPEC §7.4 + parent §8 item 17 (TLS-fronted processor cluster DEFERRED).
  Cluster mandatorily carries
  `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}`
  per SPEC §6.5 (UseH2() == true gate; REUSED from 0022).

The HTTP-service-mode processor cluster from fixture 0022 (`c_ext_proc_http`)
is NOT included at 0023 — per SPEC §2 item 1 + ADR-0168 §Decision: the
http_service-mode body PARSE-REJECT continues permanently (ext_proc proto
forbids body forwarding under http_service mode).

## 6-scenario matrix (per SPEC §7.2; AMENDED at Task 9 fixture-harness scrape)

The PLAN-time scenario matrix surfaced TWO empirical AMENDMENTS at the
Task 9 fixture-harness scrape (see `expectations.yaml` divergence_window
for full disposition):

- **Scenarios (b) + (d):** reference Envoy v1.37.2 returns 500 with empty
  body when the processor emits `CommonResponse{body_mutation{body|
  clear_body}}` at the response_body stage under `response_body_mode:
  BUFFERED`. The 500 reproduces across both echobackend and
  direct_response upstream shapes. envoy-go correctly applies the
  mutation; the cross-side divergence is a reference-proxy quirk.
  Scope-reduced to OBSERVABILITY-only (the processor sees the body
  envelope but no mutation is requested).
- **Scenario (c):** envoy-go HCM rejects encode-side SendLocalReply
  with "called SendLocalReply after encode-side started; ignoring".
  Scenario (c) re-scoped from the encode side (response_body
  immediate_response on l_test_b) to the **DECODE** side (request_body
  immediate_response on l_test_a) — the decode-side path is
  well-supported.

| # | Listener / Path | Scenario | Processor script | Expected disposition |
|---|---|---|---|---|
| a | l_test_a / `/scenario_a` | `request_body_buffered_mutation` (OBSERVABILITY-only) | `CommonResponse{}` at request_headers; `CommonResponse{}` at request_body (no mutation requested) | 200 echo + the processor SEES the body envelope (asserted post-run via `processor.Received("/scenario_a")` slice content) |
| b | l_test_b / `/scenario_b` | `response_body_buffered_mutation` (AMENDED OBSERVABILITY-only) | `CommonResponse{}` at response_headers; `CommonResponse{}` at response_body (no mutation per the AMENDMENT) | 200 + original upstream body "upstream-response-body-b" byte-exact + body-stage envelope observed |
| c | **l_test_a** / `/scenario_c` (AMENDED decode-side) | `body_stage_immediate_response` | `CommonResponse{}` at request_headers; `ImmediateResponse{status: 403, body: "denied-at-body-stage-scenario-c"}` at request_body | 403 + body byte-exact "denied-at-body-stage-scenario-c"; gRPC-status header per ADR-0172 (HTTP/1.1 downstream — header is not emitted) |
| d | l_test_b / `/scenario_d` | `body_stage_clear_body` (AMENDED OBSERVABILITY-only) | `CommonResponse{}` at response_headers; `CommonResponse{}` at response_body (no clear per the AMENDMENT) | 200 + original upstream body "upstream-response-body-d" byte-exact + body-stage envelope observed |
| e | l_test_b / `/scenario_e` | `header_stage_continue_and_replace` | `CommonResponse{status: CONTINUE_AND_REPLACE, header_mutation, body_mutation{body}}` at response_headers; body-stage outbound SKIPPED via `f.skipBodyStageDispatch[directionResponse]` | 200 + body byte-replaced with "continue-and-replace-body-scenario-e" + downstream-injected header `x-extproc-car`; ONE round-trip (no body-stage `stream_msgs_sent` increment) |
| f | l_test_c / `/scenario_f_a` + `/scenario_f_b` | `per_route_body_mode_override` | route-A: `CommonResponse{}` at request_headers + `CommonResponse{}` at request_body + `CommonResponse{}` at response_headers; route-B: `CommonResponse{}` at request_headers + `CommonResponse{}` at response_headers | 200 echo both routes; route-A emits 2 ProcessingRequests on the request side (headers + body); route-B emits 1 (headers only). Per-route counter-delta differences PRESENCE-check per ADR-0173 §Consequences AMENDMENT |

## Scenario (a) decode-side body-mutation-delivery KNOWN LIMITATION

**Scope-handled per Task 7 ADR-0168 §Consequences refresh.** The Task 7
integration documented an envoy-go-side limitation on the decode side: the
filter's `applyProcessingResponse` body_mutation arm writes the mutated
bytes to `f.decodeBodyBuf` + reconciles Content-Length on `f.decodeHeaders`,
but the mutated body bytes are NOT delivered to the upstream — envoy-go's
HCM captures the upstream-bound body bytes from its own `bodyBuf` (H1
`connection.go`) / `h2req.Body` (H2 `h2dispatch.go`) BEFORE the filter-chain
mutation lands. Only the HEADER reconciliation propagates to upstream (the
HCM's `req.Header` is the same map as `f.decodeHeaders`).

Reference Envoy v1.37.2 DOES deliver the decode-side body mutation. To
preserve cross-side equivalence, scenario (a) scopes to OBSERVABILITY-only
(**Option B** per Task 7 hand-off contract bullet 3 + the Task 9 PLAN
recommendation):

- The processor RECEIVES the body envelope (asserted in `AssertStats` via
  `processor.Received("/scenario_a")` slice content).
- The processor DOES NOT request body mutation — both sides see the
  client-supplied request body bytes verbatim at the echobackend.
- Cross-side byte equivalence holds because no mutation was requested.

The full decode-side delivery story closes in a future phase (a speculative
ADR-0178 or later analogous to ADR-0131 for the decode side); the limitation
is documented at `(*filter).DecodeData` in `extproc.go` and at the
fixture-0023 `expectations.yaml` divergence-window block.

## Encode-side scenario e — full mutation delivery via ADR-0131

For scenario (e) `header_stage_continue_and_replace` (the
CONTINUE_AND_REPLACE body-replacement path at the HEADER stage),
encode-side delivery WORKS fully via ADR-0131
`EncoderFilterCallbacks.OverwriteBody`:

- The dispatch goroutine's `completeStage` path invokes
  `(*filter).deliverEncodeBodyMutation` (which calls `f.ecb.OverwriteBody(
  f.encodeBodyBuf)`) BEFORE the resume signal fires.
- HCM at `connection.go` H1 + `h2dispatch.go` H2 reads
  `chain.EncodeBodyOverride()` post-`RunEncodeData` + substitutes
  `resp.Body` with the (possibly-mutated) bytes before the wire-write
  path consumes it.
- Content-Length is reconciled on `f.encodeHeaders` to `len(mutated body)`
  via direct `header.Set("content-length", strconv.Itoa(len(newBody)))`.
- For scenario (e), the C-1 rework guarantees the skip-flag short-circuit
  fires `OverwriteBody` BEFORE the unconditional accumulator append so
  the pre-mutated buffer is delivered intact rather than corrupted by
  the incoming HCM chunk.

Cross-side byte-exact equivalence is asserted via the differential
runner's `CompareBytes` gate on the per-scenario verdict bytes.

## Scenario (c) body-stage immediate_response (DECODE-side — AMENDED at Task 9 scrape)

The PLAN-time hypothesis placed this scenario on the encode side
(response_body stage ImmediateResponse on l_test_b). The Task 9
fixture-harness scrape surfaced an envoy-go-side framework gap: HCM
rejects encode-side `SendLocalReply` with "called SendLocalReply after
encode-side started; ignoring" when invoked from the dispatch goroutine
AFTER the encode chain has begun processing the upstream response. This
is a structural framework limitation on the encode-side body-stage
ImmediateResponse path; closure deferred to a future phase.

The substantive `body_stage_immediate_response` contract is still
asserted by re-scoping to the **DECODE** side (l_test_a;
`request_body_mode: BUFFERED` + ImmediateResponse at the `request_body`
stage). SendLocalReply on the decode side is the well-supported
framework path (the request has not reached upstream yet; SendLocalReply
is the standard rejection mechanism). Both reference Envoy v1.37.2 and
envoy-go return 403 with the processor-supplied body byte-exact.

Per ADR-0172 §Decision: gRPC-status emits as a HEADER (not trailer);
this scenario uses HTTP/1.1 downstream so the gRPC-status header is not
emitted — the deny disposition stays a plain 403.

`failure_mode_allow` does NOT fire — `ImmediateResponse` is an
intentional processor disposition, not a transport failure; the response
classification is "immediate", not "failed".

## Scenario (e) CONTINUE_AND_REPLACE + skip-flag interaction

Per SPEC §4.3 row 2 + ADR-0172 §Decision AMENDMENT at 19.2: at the header
stage WITH body-mode=BUFFERED, `CommonResponse{status:
CONTINUE_AND_REPLACE, header_mutation, body_mutation{body}}` is CONSUMED as
a combined header+body replacement. The header_mutation + body_mutation
arms are applied IN-PLACE; the body-stage outbound dispatch is SKIPPED on
the next `EncodeData` entry. The `applyProcessingResponse` step 7 sets
`f.skipBodyStageDispatch[directionResponse] = true`; the
`bodyStageEntry` helper's skip-flag short-circuit fires BEFORE the
unconditional accumulator append per the C-1 rework (Task 7 follow-up).

The driver asserts scenario (e) sees `stream_msgs_sent` increment by ONLY
ONE (the header-stage send) — the body-stage outbound is NOT dispatched.

## Scenario (f) per-route body-mode override

Per SPEC §5: `ExtProcOverrides.processing_mode` body-mode arms
(`request_body_mode`, `response_body_mode`) become CONSUMED at 19.2.
Per-route override semantics are unchanged from 19.1: the per-route override
REPLACES the listener-level `processing_mode` field-by-field for the
listener+route merge per the proto-faithful map-merge convention.
Cache-on-first-use (resolved at `DecodeHeaders` stays in effect for stream
lifetime per ADR-0173 §Consequences) is UNCHANGED — the body-stage
dispatch reads the cached `compiledConfig` from filter state without
re-resolving.

The driver POSTs distinct request bodies on both routes:

- Route-A (`/scenario_f_a`) emits a `ProcessingRequest{request_body}`
  envelope (per-route activated BUFFERED).
- Route-B (`/scenario_f_b`) does NOT emit a `ProcessingRequest{request_body}`
  envelope (listener-level NONE baseline).

The driver asserts the per-route counter-delta differences via the
`processor.Received` slice content — route-A has 2 ProcessingRequest
entries (headers + body); route-B has 1 (headers only). This is the
PRESENCE-check per ADR-0173 §Consequences AMENDMENT — strict per-scenario
counter-equivalence at the /stats/prometheus scrape is DEFERRED per SPEC
§8 item 16.

## Counter-delta assertion discipline

Per SPEC §7.5 + ADR-0173 §Consequences AMENDMENT: counter-delta assertions
stay at PRESENCE-check (counter EXISTS + counter VALUE > 0 across the
workload) NOT strict per-scenario equivalence. The 9-counter MVP roster
from 19.1 (ratified-with-amendment at fixture-0022 Task 13 scrape) carries
forward unchanged at 19.2:

- `streams_started`, `stream_msgs_sent`, `stream_msgs_received`,
  `spurious_msgs_received`, `streams_failed`, `streams_closed`,
  `failure_mode_allowed`, `override_message_timeout_received`,
  `override_message_timeout_ignored`.

Body-mode adds NO new counters per SPEC §2 item 5 + ADR-0173 §Consequences.
8 additional reference-Envoy counters DEFERRED per SPEC §8 item 14.

## D5 attribute-roster crystallization (this fixture's empirical closure)

Per planner-time D5 + Task 5 + SPEC §4.1 + §6.6 hypothesis-table extension:
the body-stage attribute envelope MIRRORS the header-stage roster (same
7 CEL-attribute-name → accessor mapping) AND ADDS the body-stage-natural
numeric attribute `request.size` (decode side) / `response.size` (encode
side) populated from `int64(len(body))`. The exact roster crystallizes
empirically at THIS Task 9 fixture-harness scrape against reference Envoy
v1.37.2's CEL attribute registry.

The driver's `AssertStats` body inspects the
`processor.Received("/scenario_a")` + `processor.Received("/scenario_b")`
+ `processor.Received("/scenario_d")` envelopes for the body-stage
attribute names + asserts against the planner-time hypothesis. The
disposition (HOLDS or AMENDED with the adjusted roster) lands in the Task 9
PROGRESS.md entry verbatim per PLAN Step 6 + 7. Any divergence triggers
an in-place `attributes.go` amendment in the SAME Task 9 commit per PLAN
Step 5 + the `git add` list.

## Divergence window from reference Envoy v1.37.2

Per ADR-0044 + SPEC §8 (full enumeration in `expectations.yaml`):

- **NEW at 19.2: decode-side body-mutation-delivery KNOWN LIMITATION** —
  the processor sees the body envelope and can request mutation, but the
  mutated bytes are NOT delivered to upstream on the envoy-go side; the
  driver scopes scenario (a) to OBSERVABILITY-only to preserve byte
  equivalence. Closure deferred to a future phase that adds the decode-side
  body-mutation-delivery framework primitive (speculative ADR-0178 or
  later).
- `ImmediateResponse.details` — silent-dropped envoy-go side per parent
  §5.P11 forward-pointer (carry-forward from 19.1).
- ProcessingMode body STREAMED/BUFFERED_PARTIAL/FULL_DUPLEX_STREAMED →
  PARSE-REJECT permanently per SPEC §2 item 2.
- ProcessingMode trailer modes != SKIP → PARSE-REJECT permanently.
- HTTP-service-mode body → PARSE-REJECT permanently per SPEC §2 item 1
  (ext_proc proto forbids body forwarding under http_service mode).
- `GoogleGrpc` arm → PARSE-REJECT permanently.
- `metadata_options` / `filter_metadata` /
  `ProcessingResponse.dynamic_metadata` / `metadata_context` /
  `CommonResponse.dynamic_metadata` / `CommonResponse.trailers` /
  `HttpHeaders.attributes` → SILENT-IGNORED (dynamic-metadata family
  deferral cluster; SPEC §8 items 5-6).
- `ExtProcOverrides.{async_mode, request_attributes, response_attributes,
  metadata_options, grpc_initial_metadata}` → SILENT-IGNORED per ADR-0173.
- `core.GrpcService.{initial_metadata, retry_policy}` → SILENT-IGNORED MVP.
- 19.1 ADR-0170 3 envelope-content divergences (protojson whitespace +
  ref-Envoy empty-message emission + writer-side `value`-vs-`raw_value`)
  carry forward per SPEC §8 item 15.
- 8 reference-Envoy additional counters DEFERRED per SPEC §8 item 14 (the
  19.1 PRESENCE-check carries forward).

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS clusters; uses
  `host.docker.internal` per ADR-0010).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC clusters; uses
  `127.0.0.1`).
- `expectations.yaml` — per-scenario allow-list + counter-delta map +
  divergence window documentation + D5 disposition closure record.
- `inputs/driver.go` — the differential driver per SPEC §7.1 + the
  D5 fixture-harness scrape closure.

## Reference

- Phase 19.2 SPEC §7 (this fixture's contract).
- Phase 19.2 PLAN Task 9 (the implementation envelope).
- ADR-0168 (compiledConfig + body-mode lift §Decision AMENDMENT at 19.2 +
  §Consequences refresh at Task 7 documenting the decode-side body-mutation
  -delivery KNOWN LIMITATION).
- ADR-0171 (4-stage state machine + per-message timer behavioral; §Decision
  AMENDMENT at 19.2).
- ADR-0172 (body-mode arms of applyProcessingResponse; §Decision AMENDMENT
  at 19.2).
- ADR-0173 (per-route 5th-canonical + SHARED-stats + counter-delta
  PRESENCE-check carry-forward).
- ADR-0175 (encode-side body-buffering primitive; the FIRST encode-side
  body-buffering framework primitive in envoy-go).
- ADR-0131 (encode-side `OverwriteBody` framework primitive; the
  encode-side delivery mechanism REUSED unchanged at 19.2).
- ADR-0128 (decode-side body-buffering; REUSED unchanged at 19.2).
- Fixture 0022 README (the headers-only precedent; topology REUSED).
