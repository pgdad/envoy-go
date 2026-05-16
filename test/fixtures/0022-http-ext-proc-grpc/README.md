# Fixture 0022-http-ext-proc-grpc

Differential fixture for `envoy.filters.http.ext_proc` — headers-only slice
(phase 19.1). Exercises BOTH service modes (gRPC + HTTP), the two header
stages (request_headers + response_headers), the per-route 5th-canonical
discipline (disabled + processing_mode override), the failure-mode posture
(failure_mode_allow), and the §19.P7 cache-on-first-use guarantee.

Body-stage scenarios land at 19.2 in fixture `0023-http-ext-proc-body`.

## Topology

Three-listener plaintext topology per planner-time decision D13:

- `l_test_a` — failure_mode_allow:false + gRPC processor (`c_ext_proc`).
  Hosts scenarios 1+2+3+4+7+8. allow_mode_override:true enables scenario
  4's mode_override response_header_mode:SKIP escape.
- `l_test_b` — failure_mode_allow:true + gRPC processor. Hosts scenario 5
  (driver stops the processor server before the request to force a
  transport failure).
- `l_test_c` — HTTP-service mode. Hosts scenario 6. Per the ext_proc proto
  constraint, http_service forbids body-stage forwarding — headers-only is
  the natural fit. The driver wires a tiny in-process HTTP server that
  reads protojson-encoded ProcessingRequest bodies and writes
  protojson-encoded ProcessingResponse bodies.

## Clusters

- `c_backend` — echobackend subprocess (HTTP/1.1) at
  `{{.BackendHost}}:{{.BackendPort}}`. Reflects request method + path +
  headers as JSON for backend-arrival assertions (scenarios 1 + 4 + 8).
- `c_ext_proc` — in-process bidi-stream gRPC `ExternalProcessor.Process`
  server at `{{.ProcHost}}:{{.ProcGRPCPort}}`. Plaintext h2c (no TLS) per
  SPEC §7.2 + parent §8 item 17 (TLS-fronted processor cluster DEFERRED).
  Cluster mandatorily carries
  `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}`
  per SPEC §6.5 (UseH2() == true gate).
- `c_ext_proc_http` — in-process HTTP/1.1 processor endpoint at
  `{{.ProcHost}}:{{.ProcHTTPPort}}` for scenario 6.

## 8-scenario matrix (per SPEC §7.1)

| # | Listener / Path | Scenario | Processor script | Expected disposition |
|---|---|---|---|---|
| 1 | l_test_a / `/scenario1` | gRPC allow + header set | `HeadersResponse{CommonResponse{header_mutation{set_headers}}}` at request_headers; `CommonResponse{}` at response_headers | 200 echo + injected header upstream |
| 2 | l_test_a / `/scenario2` | gRPC immediate_response at request_headers | `ImmediateResponse{status:403, body:"denied", headers}` | 403 + body byte-exact |
| 3 | l_test_a / `/scenario3` | gRPC response_headers mutation | `CommonResponse{}` at request_headers; `CommonResponse{header_mutation{set_headers}}` at response_headers | 200 echo + injected header downstream |
| 4 | l_test_a / `/scenario4` | gRPC mode_override mid-stream | `CommonResponse{} + mode_override{response_header_mode:SKIP}` | 200 echo; response_headers stage SKIPPED |
| 5 | l_test_b / `/scenario5` | gRPC failure_mode_allow (processor down) | processor stopped before request | 200 echo (failure_mode_allow:true allows) |
| 6 | l_test_c / `/scenario6` | HTTP allow (headers-only) | per-stage POST returns JSON ProcessingResponse | 200 echo + injected header upstream |
| 7 | l_test_a / `/disabled` | per-route disabled | (no processor invocation) | 200 echo; NO counter increments |
| 8 | l_test_a / `/override` | per-route processing_mode override | `CommonResponse{header_mutation{set_headers}}` at response_headers (request stage SKIPPED by per-route mode override) | 200 echo + injected header downstream; cache-on-first-use confirmed |

## Per-route 5th-canonical discipline (REUSED per ADR-0173)

Phase 19.1 REUSES the existing 5th canonical per-route TPFC pattern (no
ADR-0125 §(xiv) amendment fires — this is the SECOND CONSECUTIVE §9 row to
REUSE after phase 18; ADR-0173 records the explicit no-amendment decision).
Scenarios 7 + 8 exercise the two arms:

- Scenario 7 (`/disabled`) — `ExtProcPerRoute{disabled: true}` bypasses the
  filter entirely. Cross-side equivalence assertion: zero ext_proc counter
  increments on either side.
- Scenario 8 (`/override`) — `ExtProcPerRoute{overrides{processing_mode}}`
  overrides the listener-level mode. Cross-side equivalence assertion: the
  overridden mode applies at the request_headers stage (SKIPPED) AND at
  the response_headers stage (SEND).

## SHARED-stats discipline

All three listeners (l_test_a/b/c) emit ext_proc counters under their
own per-HCM stat-prefix namespace (`http.hcm_local_<X>.ext_proc.*`). The
driver scrapes /stats/prometheus from both admin endpoints AFTER the
8-scenario workload and asserts cross-side equivalence on the reachable
counter deltas + the 9-counter hypothesis verbatim match.

## Counter-delta assertion discipline

Per SPEC §15 item 9 — per-scenario byte-equivalent body + status + header
+ counter-delta equivalence. The driver scrapes /stats/prometheus from
both sides AFTER the 8-scenario workload and asserts the cumulative
counter deltas match the 9-counter hypothesis:

- `streams_started`           = 6  (scenarios 1, 2, 3, 4, 5 (fails), 8; scenarios 6 + 7 not gRPC)
- `stream_msgs_sent`          = 8  (S1:2, S2:1, S3:2, S4:1, S8:1, S6:2 — S6 in HTTP-mode also publishes the stream-msg counters)
- `stream_msgs_received`      = 8  (same as sent)
- `streams_closed`            = 6
- `streams_failed`            = 1  (S5 — gRPC dial fails)
- `failure_mode_allowed`      = 1  (S5 — failure_mode_allow:true)
- `spurious_msgs_received`    = 0  (no protocol violations on happy path)
- `override_message_timeout_received` = 0
- `override_message_timeout_ignored`  = 0

Exact hypothesized values may be amended in-place at the Task 13 IMPL
fixture-harness scrape if cross-side delta divergence surfaces.

## Divergence window from reference Envoy v1.37.2

Per ADR-0044 + SPEC §8:

- `ImmediateResponse.details` — silent-dropped envoy-go side per parent
  §5.P11 forward-pointer to a future response_code_details framework
  primitive (jointly blocked across phases 16+17+18+19).
- `CONTINUE_AND_REPLACE` (CommonResponse.status) — classified as
  spuriousMsgsReceived++ in 19.1 per planner-time D7; lifts at 19.2 once
  body-mode activates.
- body-modes != NONE → PARSE-REJECT in 19.1 (lifts at 19.2 per AMENDMENT).
- trailer-modes != SKIP → PARSE-REJECT permanently.
- STREAMED-only flags → PARSE-REJECT permanently.
- `GoogleGrpc` arm → PARSE-REJECT permanently.
- `metadata_options` / `filter_metadata` / `ProcessingResponse.dynamic_metadata`
  / `ProcessingRequest.metadata_context` / `CommonResponse.dynamic_metadata`
  / `CommonResponse.trailers` / `HttpHeaders.attributes` — SILENT-IGNORED
  per the dynamic-metadata family deferral cluster.
- `ExtProcOverrides.{async_mode, request_attributes, response_attributes,
  metadata_options, grpc_initial_metadata}` — SILENT-IGNORED per ADR-0173.
- `core.GrpcService.{initial_metadata, retry_policy}` — SILENT-IGNORED MVP.

## ADR-0174 encode-side callback symmetry landed

Phase 19.1 is the FIRST cross-phase consumer of the ADR-0174
`EncoderFilterCallbacks` 6-method extension (Task 5). The
`response_attributes` envelope population at response_headers stage uses
the new encode-side accessors — this fixture's scenario 3 + scenario 6
exercise the encode-side path.

## Three RATIFIED-PENDING-IMPL-TIME pin closures captured by the driver

Per parent SPEC §5.P4 + §5.P7 + §5.P8:

- **§19.P4** — 9-counter stat-surface roster + canonical names. The
  driver scrapes /stats/prometheus from both sides post-scenario-1 and
  asserts the 9 counter NAMES match the hypothesis verbatim under
  `http.hcm_local_a.ext_proc.*`. RATIFIED at fixture-harness scrape OR
  ADR-0173 §Decision in-place AMENDMENT.
- **§19.P7** — cache-on-first-use per-route after ClearRouteCache.
  Scenario 8 (`/override`) carries TWO assertions: the per-route
  processing_mode override applies (primary scenario contract) AND the
  cache-on-first-use guarantee holds across hypothetical mid-stream
  ClearRouteCache invocations. RATIFIED OR ADR-0173 §Decision AMENDMENT.
- **§19.P8** — JSON codec wire-shape vs `protojson` defaults. Scenario 6
  driver captures one ProcessingRequest POST body + one ProcessingResponse
  response body and asserts byte-equivalent protojson rendering against
  reference Envoy v1.37.2. RATIFIED OR ADR-0170 §Decision AMENDMENT.

## Files

- `envoy.yaml` — reference Envoy bootstrap (STRICT_DNS clusters; uses
  `host.docker.internal` per ADR-0010).
- `envoy-go.yaml` — envoy-go bootstrap (STATIC clusters; uses
  `127.0.0.1`).
- `expectations.yaml` — per-scenario allow-list + counter-delta map +
  divergence window documentation.
- `inputs/driver.go` — the differential driver per SPEC §7.1 + the 3 pin
  closures.

## Reference

- Parent phase 19 SPEC §5.P4 / §5.P7 / §5.P8 (the 3 RATIFIED-PENDING-IMPL-TIME pins).
- Phase 19.1 SPEC §7 (this fixture's contract).
- ADR-0167, ADR-0168, ADR-0169, ADR-0170, ADR-0171, ADR-0172, ADR-0173,
  ADR-0174 (the 8 ADRs landed at 19.1 IMPL).
