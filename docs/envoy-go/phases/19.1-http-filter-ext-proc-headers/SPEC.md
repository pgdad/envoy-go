# Phase 19.1 SPEC — `envoy.filters.http.ext_proc` (filter scaffold + headers stages + bidi-stream primitive)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `19.1` added `in-progress` at this SPEC commit (parent row `19` flips `planned → in-progress`; row `19.2` added `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–18.x precedent. This SPEC is the authoritative input to the 19.1 PLAN.

**Parent:** `docs/envoy-go/phases/19-http-filter-ext-proc/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4, the **full §5 13-pin empirical-pin block** resolved IN-SESSION, the §6 empirical-finding amendment block, the §7 10-ADR anchor map, and the §3 ADR-0045 split rationale). This sub-phase SPEC details the 19.1 surface only; it REFERENCES the parent's §5/§6/§7 rather than repeating them.

**Predecessors:** `docs/envoy-go/phases/19-http-filter-ext-proc/BRAINSTORM.md` (the §10 empirical pins are resolved in the parent SPEC §5). NO off-master prebrainstorm-notes branch.

**Sub-phase scope (per parent SPEC §2):** 19.1 lands the foundational filter — the NEW `internal/filter/http/extproc/` package, the `ExternalProcessor` proto parsing, the dual-mode `compiledConfig` envelope (the `grpc_service` AND `http_service` arms are BOTH activated in 19.1; the body-mode-not-NONE arms PARSE-REJECT at 19.1 and 19.2 activates them), the bidi-stream framework primitive at `internal/grpcclient/` extension (ADR-0169 — `*ProcessorClient` alongside the unary `*AuthClient`), the JSON codec for `ProcessingRequest`/`ProcessingResponse` (ADR-0170 — filter-local), the **headers stages only** (request_headers + response_headers), the per-stage `CommonResponse.header_mutation` application, the multi-stage `ImmediateResponse` deny path (request_headers + response_headers), the ProcessingMode state machine + mode-override re-eval, the `clear_route_cache` + `route_cache_action` precedence + `cb.ClearRouteCache()` reuse, the per-route 5th-canonical REUSE + SHARED-stats + the 9-counter filter stat surface, the `failure_mode_allow` / `message_timeout` / `max_message_timeout` / `disable_immediate_response` error posture, the bidi-stream lifecycle (`OnDestroy` cancel + `CloseSend`), the async-resume outbound-call leg, boot-registration, the BOTH-DECODE-AND-ENCODE filter shape + `filterStats` + the deny-path `SendLocalReply` mechanism. **ADR-0174 lands at 19.1** — the symmetric `EncoderFilterCallbacks` extension is required for `response_attributes` population at the response_headers stage per parent §5.P12 REFUTED. **19.2 (the body-stage activation + ADR-0175 encode-side body-buffering primitive) is OUT OF SCOPE for 19.1.**

**ADR continuity:** Phase 18.2 closed at ADR-0166. Phase 19 anticipates ADR-0167..ADR-0176 (10 ADRs per parent SPEC §7). The 19.1-landing ADRs are **ADR-0167, ADR-0168 (§Decision; amended at 19.2 to lift body-mode PARSE-REJECT), ADR-0169, ADR-0170, ADR-0171 (header-mode portion; amended at 19.2 to add body-mode state-machine), ADR-0172 (header_mutation + ImmediateResponse-at-headers-stage portion; amended at 19.2 to add body_mutation + CONTINUE_AND_REPLACE + body-stage immediate_response), ADR-0173, ADR-0174** — anchored with §Context drafts at the parent SPEC commit; §Decision + §Consequences bodies LAND at each ADR's Lands-in-Task per ADR-0044. **ADR-0176** (the ADR-0045 split-application ADR) landed IN FULL at the parent SPEC commit. ADR-0175 (the encode-side body-buffering primitive) lands in 19.2. Next-free ADR after phase 19 is ADR-0177.

**Authored:** 2026-05-15.

---

## 1. Purpose

Phase 19.1 lands `envoy.filters.http.ext_proc` in **headers-stages-only mode** — the canonical Envoy external-processor filter delegating per-stage mutation decisions for the request_headers and response_headers stages to an external service over either a bidirectional gRPC `Process` stream OR per-stage HTTP-JSON POSTs — as the foundational half of the TWELFTH §9 production HTTP filter. It establishes the entire `internal/filter/http/extproc/` package, the dual-mode `compiledConfig` envelope, the bidi-stream framework primitive, the JSON codec, the per-stage state machine, the per-route discipline, and the 9-counter stat surface. **Body-stage participation (`request_body_mode`/`response_body_mode = BUFFERED`) PARSE-REJECTs in 19.1** — those arms activate at 19.2 along with ADR-0175. The seven architectural primitives:

1. **New `internal/filter/http/extproc/` package** owning the filter implementation. Package directory + Go-package identifier are both `extproc` (single token underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/` + `extauthz/`). Files mirror the phase-18.1+18.2 multi-file split: `extproc.go` (filter type + factory + decode AND encode methods + `filterStats` struct + `compiledConfig` + per-route helper), `check.go` (the per-stage dispatcher — gRPC bidi-stream `Process` send/recv path + HTTP POST `ProcessingRequest`/`ProcessingResponse` path + the `CommonResponse` mutation-application logic + the `ImmediateResponse` deny-emission logic + `failure_mode_allow` / `message_timeout` / `max_message_timeout` handling), `attributes.go` (the `request_attributes` + `response_attributes` allowlist-driven attribute envelope builder; populates source/destination address attributes + TLS-attribute set per the proto's CEL-attribute-name allowlist via the ADR-0165 + ADR-0174 callback accessors), `processor.go` (the bidi-stream state machine: per-direction ProcessingMode state, mode-override re-evaluation, the single-in-flight-message correlation discipline, the per-stage timer with override_message_timeout extension support, `OnDestroy`-driven cancel + `CloseSend` lifecycle), `json.go` (the filter-local `protojson` codec for http_service mode — `marshalProcessingRequest` + `unmarshalProcessingResponse`), `extproc_test.go` (unit tests; anticipated 1500–2500 LoC given the dual-mode + bidi-stream + state-machine + mutation subsurface — 19.1 portion), `fuzz_test.go` (the 24th fuzzer — `FuzzExtProcConfigParse`), `doc.go` (package overview + the 6-decision summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"`) + `New` (the `HTTPFilterFactory`) per the cors/fault/.../extauthz precedent. ADR-0167 codifies.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 14 entries after phase 18.2: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `extauthz.New`, `fault.New`, `header_mutation.New`, `jwtauthn.New`, `localratelimit.New`, `rbac.New` before `httpReg.Freeze()`) gains a fifteenth `httpReg.Register(extproc.TypeURL, extproc.New)` call before the freeze. Insertion alphabetical-after-router per ADR-0100 §2.2: `extproc` inserts between `extauthz` and `fault`. Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only.

3. **`ExternalProcessor` proto parsing — the dual-mode `compiledConfig` envelope.** Per parent SPEC §4.1 + §4.2 + ADR-0168: `grpc_service` and `http_service` are top-level optional fields (NOT a proto oneof per parent §5.P1); the factory enforces mutual-exclusion at parse time (both-set OR neither-set → PARSE-REJECT envoy-go-strict). In 19.1 BOTH arms are activated: `grpc_service` builds a `*grpcclient.ProcessorClient` bidi-stream wrapper; `http_service` builds an HTTP-JSON per-stage POST client. **Body-mode PARSE-REJECT**: in 19.1, `processing_mode.request_body_mode` and `response_body_mode` PARSE-REJECT anything other than NONE (the BUFFERED arms activate at 19.2; STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED PARSE-REJECT envoy-go-strict permanently). **Trailer-mode PARSE-REJECT**: anything other than SKIP PARSE-REJECTs envoy-go-strict (out of envelope per Q2). **STREAMED-only flag PARSE-REJECT**: `observability_mode=true`, `send_body_without_waiting_for_header_response=true`, `deferred_close_timeout` non-zero PARSE-REJECT envoy-go-strict (out of envelope per Q2 + parent §5.P10). The 19.1 MVP consumes (per parent §5.P1): `grpc_service` (#1, with `core.GrpcService.GoogleGrpc` PARSE-REJECT + `initial_metadata`/`retry_policy` SILENT-IGNORE per parent §4.3), `http_service` (#20 → `ExtProcHttpService.http_service: *core.v3.HttpService`), `failure_mode_allow` (#2), `processing_mode` (#3 — headers-modes consumed, body/trailer-modes PARSE-REJECTed-or-restricted as above), `request_attributes` (#5), `response_attributes` (#6), `message_timeout` (#7; default 200ms; PGV `gte=0s, lte=1h0m0s`), `max_message_timeout` (#10; default 0; PGV `gte=0s, lte=1h0m0s`), `mutation_rules` (#9), `forward_rules` (#12), `allow_mode_override` (#14), `disable_immediate_response` (#15), `route_cache_action` (#18) + `disable_clear_route_cache` (#11; mutually exclusive with #18 per parent §5.P5), `stat_prefix` (#8), `allowed_override_modes` (#22). DEFERRED per parent §4.4 + §8 below. ADR-0168 codifies the envelope.

4. **NEW `*ProcessorClient` bidi-stream wrapper EXTENDING `internal/grpcclient/` (ADR-0169).** A NEW typed wrapper alongside the existing unary `*AuthClient` in the SAME `internal/grpcclient/` package — NOT a new framework family. Public surface (the IMPL settles the exact signature):

   ```go
   // ProcessorClient wraps the typed envoy.service.ext_proc.v3.ExternalProcessorClient stub.
   type ProcessorClient struct { /* ... */ }

   // NewProcessorClient dials the named cluster and returns a typed wrapper.
   // Reuses the existing *Dialer (no Dialer API changes).
   func NewProcessorClient(d *Dialer, clusterName string, perMessageTimeout time.Duration) (*ProcessorClient, error)

   // Process opens a new bidi-stream Process RPC. The stream lifetime is the
   // per-HTTP-transaction state machine in extproc/processor.go.
   func (c *ProcessorClient) Process(ctx context.Context) (ProcessStream, error)

   // ProcessStream is the bidi-stream client-side interface.
   type ProcessStream interface {
       Send(*extprocv3.ProcessingRequest) error
       Recv() (*extprocv3.ProcessingResponse, error)
       CloseSend() error
   }

   // Close releases the underlying *grpc.ClientConn.
   func (c *ProcessorClient) Close() error
   ```

   Composes against the existing `*Dialer` via `Dialer.DialContext(ctx, clusterName)` → wraps the returned `*grpc.ClientConn` with `envoy.service.ext_proc.v3.NewExternalProcessorClient(conn)`. One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair created at config-load time (mirrors the phase-18.2 `*AuthClient` lifecycle per ADR-0158 §Decision (v)). Bidi-stream lifetime is per HTTP transaction (one `Process` stream per request); the `*grpc.ClientConn` is reused across streams via gRPC's internal multiplexing. Cross-phase-reusable for any future bidi-stream gRPC filter (e.g., a future streaming-access-log filter; ext_proc is currently the sole bidi-stream consumer). ADR-0169 anchors.

5. **NEW `ProcessingRequest`/`ProcessingResponse` JSON codec (ADR-0170 — filter-local for MVP).** The http_service arm requires JSON marshalling of `ProcessingRequest` and unmarshalling of `ProcessingResponse` over an HTTP POST per stage. The codec uses `protojson` (already in the dependency tree via go-control-plane). Wire-shape: a single JSON-encoded `ProcessingRequest` per POST body; a single JSON-encoded `ProcessingResponse` per HTTP response body. Per parent §5.P8 RATIFIED-PENDING-IMPL-TIME: `protojson` defaults (snake_case field names; well-known type formatting; enum-as-string; empty-fields omitted) are the hypothesis; the IMPL fixture-harness scrape against reference Envoy v1.37.2 closes the pin RATIFIED. The codec lives filter-local in `extproc/json.go` — minimal public surface (`marshalProcessingRequest(*ProcessingRequest) ([]byte, error)` + `unmarshalProcessingResponse([]byte) (*ProcessingResponse, error)`). Filter-local for MVP per ADR-0170 §Decision; generalization to a shared `internal/jsoncodec/` package deferred to the second-consumer trigger per the phase-18.1 ADR-0159 (b)-disposition rationale (the natural trigger is a future filter needing protojson over HTTP — currently no other in-tree consumer). **HTTP-service-mode is constrained to headers-only per the proto's `ExtProcHttpService.http_service` doc-comment** (quoted at parent §5.P1: "if 'http_service' is set, the `processing_mode` can not be configured to send any body or trailers"); the IMPL enforces this at parse time when `http_service` is set + `request_body_mode`/`response_body_mode` ≠ NONE → PARSE-REJECT. ADR-0170 anchors.

6. **Bidi-stream state machine + per-stage dispatcher (`check.go` + `processor.go`) — ADR-0171 + ADR-0172 (header-mode portions).** Per parent §5.P9 + §5.P10. The state machine carries per-direction `ProcessingMode` + the current outstanding stage. Stages fire in proto-faithful order: request_headers → (body PARSE-REJECT in 19.1; lifts at 19.2) → upstream → response_headers → (body PARSE-REJECT in 19.1; lifts at 19.2) → downstream. Each stage's `ProcessingRequest` carries the populated set (`headers` for headers stages; `attributes` map populated per the `request_attributes`/`response_attributes` allowlist — at request_headers from ADR-0165 decode-side accessors; at response_headers from the NEW ADR-0174 encode-side accessors). Each `ProcessingResponse` carries a `HeadersResponse{response: CommonResponse}` or an `ImmediateResponse{status, headers, body, grpc_status, details}` or a `mode_override`+other-fields-ignored response. Single-in-flight: at most one outstanding stage at any time (no STREAMED interleaving). After each `ProcessingResponse`, the filter re-evaluates ProcessingMode (per parent §5.P1 — header-response paths only) when `allow_mode_override=true` and `mode_override` is in `allowed_override_modes`. `mutation_rules` gates header_mutation per-header (parent §5.P3); rejected mutations classify as `spurious_msgs_received` counter increment. The dispatcher is async: each stage's outbound send + recv parks the decode (request_headers) or encode (response_headers) dispatch goroutine on the per-stream resume channel (phase-09 async-resume primitive reuse) so the HCM dispatch goroutine is not blocked on network I/O. `OnDestroy` cancels the in-flight call's `context.Context` and `CloseSend`s the bidi-stream. ADR-0171 anchors the state machine; ADR-0172 anchors the CommonResponse mutation + ImmediateResponse multi-stage discipline (header-mode portion in 19.1; body-mode portion lands at 19.2).

7. **Filter-callback shape: BOTH `StreamDecoderFilter` AND `StreamEncoderFilter`** (`Decoder: non-nil`; `Encoder: non-nil`). Phase 19 is the FIRST §9 family-row to ship both — decoder side for request_headers stage; encoder side for response_headers stage. Static blank-identifier compile-time checks for BOTH interfaces. The decode-side surface: `DecodeHeaders(headers, endStream)` resolves per-route → caches `*compiledPerRoute` on filter state → if the per-route arm is `disabled` returns `HeaderContinue` immediately (no processor call, no counter increments — per parent §5.P6); else if request_header_mode ∈ {SEND, DEFAULT}, returns `HeaderStopIteration` and fires the request_headers stage outbound call; if SKIP, returns `HeaderContinue` (skip the processor call); on processor response, applies `CommonResponse.header_mutation` via `f.dcb.RequestHeaders().*` mutators + advances ProcessingMode per `ProcessingResponse.mode_override` (when `allow_mode_override`). `DecodeData` PARSE-REJECT at compile-time per the 19.1 PARSE-REJECT-body-modes discipline (the body-mode PARSE-REJECT happens at parse time, so `DecodeData` is never called for a body-mode-not-NONE config — but `DecodeData` is implemented as a `DataContinue` pass-through in 19.1 for code-completeness; 19.2 replaces with the BUFFERED-arm body-stage logic). `DecodeTrailers` PARSE-REJECT at parse time when trailer-mode ≠ SKIP; the method itself is `TrailersContinue` pass-through. The encode-side surface mirrors decode for response_headers: `EncodeHeaders(headers, endStream)` if response_header_mode ∈ {SEND, DEFAULT} returns `HeaderStopIteration` and fires the response_headers stage; on response applies `CommonResponse.header_mutation` to response headers. `EncodeData`/`EncodeTrailers` PARSE-REJECT at parse time per the body/trailer-mode discipline. **`ImmediateResponse` at response_headers stage** — when the processor returns `ImmediateResponse` on a response_headers ProcessingRequest, the filter emits a `SendLocalReply` from the encode-side (this is a FIRST in envoy-go — multi-stage SendLocalReply with the FIRST emission from encode-side participation; the framework's `SendLocalReply` primitive per ADR-0085 supports this since it enters the encode chain at filter[len-1] per ADR-0075). ADR-0167 codifies the decoder+encoder shape; ADR-0172 anchors the multi-stage deny mechanism.

Plus the **per-route 5th-canonical REUSE + SHARED-stats + 9-counter stat surface (ADR-0173)** and the **NEW ADR-0174 encode-side callback symmetry** — detailed in §3, §5, §6 below.

After phase 19.1, the project has the foundational ext_proc filter: a both-decode-and-encode-side filter that ships request_headers + response_headers stages to an external processor over either a bidi-stream gRPC `Process` or per-stage HTTP-JSON POSTs, parks the dispatch goroutine on an async-resume leg while each stage's outbound call is in flight, applies per-stage `CommonResponse.header_mutation` set/remove with `mutation_rules` per-header gating, honors `ImmediateResponse` deny at either header stage via `SendLocalReply` with optional `grpc-status` trailers when the downstream is gRPC, re-evaluates per-direction ProcessingMode when the processor returns a `mode_override` validated against `allowed_override_modes`, and honors the `failure_mode_allow` / `message_timeout` / `max_message_timeout` / `disable_immediate_response` error posture when the processor is unreachable — observable-outcomes byte-equivalent to reference Envoy v1.37.2 headers-only ext_proc on every axis except the documented divergence-windows. Phase 19.2 then activates the BUFFERED body-stage arms + ADR-0175 against the same package surface.

### 1.1 Empirical-finding-driven scope (per parent SPEC §6)

The 12 §6 amendments in the parent SPEC are the empirical-finding-driven scope revisions for phase 19. The amendments load-bearing for 19.1: **amendment 1** (mode_override timing — header-response paths only), **amendment 2** (`ImmediateResponse.headers` is `*HeaderMutation`; gRPC-downstream-detection via content-type sniff), **amendment 3** (`mutation_rules` per-header rejection + protected default set), **amendment 4** (9-counter stat surface RATIFIED-PENDING-IMPL-TIME; 77 → ~86 names), **amendment 5** (`route_cache_action` mutual-exclusion at parse time; request-headers-stage-only), **amendment 6** (`ExtProcOverrides` 7-field roster RATIFIED — including `request_attributes`/`response_attributes` `[#not-implemented-hide:]` silent-ignore at the per-route level; the same field names exist as top-level `ExternalProcessor` fields which are MVP-consumed for listener-level envelopes), **amendment 7** (per-route cache-on-first-use after `ClearRouteCache`), **amendment 8** (JSON codec `protojson` defaults RATIFIED-PENDING-IMPL-TIME), **amendment 9** (ProcessingMode DEFAULT semantics — SEND for headers, SKIP for trailers; BodySendMode zero is NONE), **amendment 10** (bidi-stream half-close + STREAMED-only flag PARSE-REJECT + override_message_timeout + GoogleGrpc PARSE-REJECT), **amendment 12** (encode-side callback symmetry — ADR-0174 fires in 19.1). Amendment 11 (encode-side body-buffering — ADR-0175) is 19.2-only. This 19.1 SPEC's §4/§5/§6/§7 incorporate amendments 1/2/3/4/5/6/7/8/9/10/12 into the formal SPEC shape.

---

## 2. Non-purposes

Phase 19.1 is a single-sub-phase slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land headers-stages ext_proc under the existing 07.1 framework + the FOUR new ADR-anchored deltas (ADR-0169 bidi-stream wrapper, ADR-0170 JSON codec, ADR-0174 encode-side callback symmetry, ADR-0167+0173 package shape + stat surface).

- **2.1 Body-stage participation is OUT OF SCOPE.** `request_body_mode`/`response_body_mode = BUFFERED` PARSE-REJECT in 19.1. ADR-0175 (the encode-side body-buffering primitive), the body_mutation discipline, the CONTINUE_AND_REPLACE handling, and the multi-stage immediate_response at body stages all land in 19.2.
- **2.2 STREAMED body modes** (`STREAMED`, `BUFFERED_PARTIAL`, `FULL_DUPLEX_STREAMED`) PARSE-REJECT envoy-go-strict — permanently out of envelope per Q2 (NOT lifted at 19.2 either — that's a follow-up streaming-body framework phase).
- **2.3 Trailer modes** (`request_trailer_mode`/`response_trailer_mode` ≠ SKIP) PARSE-REJECT envoy-go-strict — permanently out of envelope per Q2.
- **2.4 STREAMED-only flags** (`observability_mode`, `send_body_without_waiting_for_header_response`, `deferred_close_timeout` non-zero) PARSE-REJECT envoy-go-strict per parent §5.P10 — permanently out of envelope per Q2.
- **2.5 Deferred `ExternalProcessor` fields** (per parent §4.4 + §8 below): `metadata_options` (#16), `filter_metadata` (#13), `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` (5 of the 7 ExtProcOverrides fields), `ProcessingResponse.dynamic_metadata`, `ProcessingRequest.metadata_context` (populated as empty proto for forward-compat), `CommonResponse.dynamic_metadata`-style emit, `CommonResponse.trailers` (`[#not-implemented-hide:]`), `HttpHeaders.attributes` (deprecated + `[#not-implemented-hide:]`).
- **2.6 GoogleGrpc arm + gRPC retry-policy customization** — `core.GrpcService.GoogleGrpc` PARSE-REJECT envoy-go-strict per ADR-0157 §Decision AMENDMENT (inherited from phase-18.2). `core.GrpcService.initial_metadata` + `core.GrpcService.retry_policy` SILENT-IGNORED per the phase-18.2 SPEC §2.6 carry-forward.
- **2.7 No filter-chain ordering surgery.** ext_proc registers as one more entry in the existing extension registry; the HCM filter-chain iteration protocol is unchanged.
- **2.8 No `response_code_details` emission** — unchanged from phase-16/17/18.x; ext_proc's deny-path `ImmediateResponse.details` field would map to `response_code_details` but envoy-go's HCM does not surface this (phase-04 scope) — documented divergence-window joint with phase-16/17/18 forward-pointers.
- **2.9 No xDS-driven dynamic processor-cluster reconfiguration.** The fixture uses static cluster config; xDS-CDS replacement of the processor cluster is not exercised (envoy-go's MVP cluster manager is static-config-only as of master tip). Future xDS-CDS phase covers.
- **2.10 No new SN-flattening rule** — the SN2-reuse hypothesis (parent §5.P4) is RATIFIED-PENDING-IMPL-TIME; the 19.1 IMPL fixture-harness empirical confirmation closes it. If refuted, an IMPL-time ADR-0044 escape-valve fires.
- **2.11 No `dynamic_metadata` consumption** (the `CommonResponse.dynamic_metadata` flag if it existed, OR `ProcessingResponse.dynamic_metadata`-style emit) — DEFERRED per dynamic-metadata family cluster.

---

## 3. Framework survey result (TWO NEW primitives + ONE NEW codec + ONE NEW (callback symmetry) in 19.1 + FOUR reuses; phase-13 ADR-0128 reuse deferred to 19.2)

The framework survey evaluated reuse of phase-09-through-18.2 primitives BEFORE proposing new (per the phase-16/17/18.x discipline). Findings for 19.1:

- **Phase-09 `time.AfterFunc` + `cb.ContinueDecoding`/`cb.ContinueEncoding` async-resume primitives**: **REUSED** — per-stage outbound calls park decode/encode dispatch goroutines on resume channels. Same primitive fault introduced; ext_proc is the FIRST cross-phase consumer for the ENCODE-SIDE async-resume leg (phase-09 + phase-18.1+18.2 + phase-14 all exercise decode-side async-resume; encode-side participation alongside decode-side is new).
- **Phase-13 ADR-0128 decode-side body-buffering primitives**: NOT reused in 19.1 (body-stage activation is 19.2's scope). Reuse at 19.2.
- **Phase-14 ADR-0131 `EncoderFilterCallbacks.OverwriteBody`**: NOT reused — `OverwriteBody` is a per-call replacement primitive (called from inside `EncodeData`); it is NOT a buffer-and-hold primitive. Per parent §5.P11 the encode-side BUFFERED body-mode requires a NEW primitive (ADR-0175 at 19.2), not `OverwriteBody`.
- **Phase-16 ADR-0142 matcher-engine at `internal/matcher/`**: NOT reused (ext_proc's per-route is the simple 5th-canonical oneof; no Matcher-typed field).
- **Phase-16 ADR-0144 `DownstreamPrincipal()`**: REUSED INDIRECTLY via the 6 ADR-0165 methods. The `request_attributes` envelope (parent §5.P1 + the proto's CEL-attribute-name allowlist) consumes `DownstreamPrincipal()`-derived attribute values + the 6 socket/TLS accessors.
- **Phase-18.2 ADR-0158 `internal/grpcclient/Dialer`**: **REUSED LOAD-BEARING** — the FIRST cross-phase consumer of ADR-0158 outside phase-18.2 itself. The `*ProcessorClient` (ADR-0169) is composed alongside the unary `*AuthClient` using the same `Dialer` (`NewProcessorClient(d *Dialer, ...)`). NO `Dialer` API changes (the ADR-0158 §Consequences explicitly anchored this cross-phase shape: "no future client coupling is anticipated to require `Dialer` API changes").
- **Phase-18.2 ADR-0165 6 new `DecoderFilterCallbacks` methods**: **REUSED LOAD-BEARING** — the FIRST cross-phase consumer of ADR-0165. The `request_attributes` envelope populates from these accessors at `DecodeHeaders` time. The 6 methods: `DownstreamRemoteAddr` / `DownstreamLocalAddr` / `DownstreamTLSServerName` / `DownstreamTLSPeerCertDER` / `DownstreamProtocol` / `ListenerPrincipal`. Plus the existing `DownstreamPrincipal` (ADR-0144) for principal-derived attributes.
- **Phase-18.2 ADR-0166 cluster-manager plaintext h2c upstream relaxation**: **REUSED LOAD-BEARING** for the fixture-0022 `c_ext_proc` test cluster (plaintext h2c upstream; mirrors fixture-0021 `c_authz_grpc`).
- **ADR-0085 `SendLocalReply` framework primitive**: **REUSED** — the deny-path emission (§4). `content-length` synthesized + the standard header set (`server: envoy`, `date`) per ADR-0085. The FIRST §9 row whose deny-path can fire from the ENCODE side (response_headers stage immediate_response) — the framework's per-ADR-0075 "SendLocalReply enters the encode chain at filter[len-1]" semantics support this.
- **ADR-0125 8 canonical per-route patterns**: NO NEW canonical — the **5th canonical is REUSED** (§5 + ADR-0173; the SECOND CONSECUTIVE §9 row after phase 18 to REUSE rather than add). NO ADR-0125 §(xiv) amendment paragraph.

**Three new deltas in 19.1**: (i) the `*ProcessorClient` bidi-stream wrapper (ADR-0169 — extends ADR-0158, NOT a new framework family); (ii) the filter-local JSON codec (ADR-0170); (iii) the symmetric `EncoderFilterCallbacks` extension (ADR-0174 — required for `response_attributes` population; §5.P12 REFUTED at SPEC time).

### 3.1 NEW: `*ProcessorClient` bidi-stream wrapper EXTENDING `internal/grpcclient/` (ADR-0169)

Per parent §3.1 + the public surface sketched at §1 item 4. The wrapper composes against the existing `*Dialer`; no `Dialer` API changes (ADR-0158 §Consequences anchored this). Public surface lives at `internal/grpcclient/processor_client.go` (NEW file alongside `auth_client.go`).

**Internal construction** mirrors the `AuthClient` pattern (per ADR-0158 §Decision (iv)): `NewProcessorClient(d *Dialer, clusterName string, perMessageTimeout time.Duration)` calls `d.DialContext(ctx, clusterName)` → `conn, err`; wraps with `extprocv3.NewExternalProcessorClient(conn)` returning the typed stub. The per-Check timeout in `AuthClient` becomes a per-MESSAGE timeout in `ProcessorClient` — each `Send`+`Recv` pair has the timeout applied via `context.WithTimeout` around the recv path (the `Send` is non-blocking on the gRPC client stream; the `Recv` blocks until the processor responds). The per-message timer interacts with `override_message_timeout` per parent §5.P10 — the IMPL resets the recv-timer when an `override_message_timeout` ProcessingResponse arrives.

**Connection lifecycle.** One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair. Created in `buildGRPCProcessorClient` at config-load time; the closure captures `*ProcessorClient`; shared across all per-stream `Process()` invocations (gRPC manages its own transport-level reconnect via the underlying sub-channel state machine). On process exit, the `ProcessorClient` is GC'd; `Close()` is NOT explicitly called for MVP (mirrors phase-18.2 ADR-0158 §Decision (vi) leaks-on-exit). When envoy-go gains xDS-CDS hot-reload, the close-on-replacement discipline lands per a future ADR (NOT 19.1).

**Stream lifecycle.** Per HTTP transaction (one `Process` stream per request). The state machine at `extproc/processor.go` opens a stream via `ProcessorClient.Process(ctx)` at `DecodeHeaders` time when request_header_mode ∈ {SEND, DEFAULT}; sends ProcessingRequests per stage; receives ProcessingResponses; calls `CloseSend()` after the final response stage OR on `ImmediateResponse` arrival OR on `OnDestroy` cancellation. The stream's `ctx` carries the per-stream cancellation hook so `OnDestroy` aborts in-flight `Send`/`Recv` calls.

**Cross-phase reuse intent (ADR-0169 §Consequences).** Future bidi-stream gRPC filters reuse the `ProcessorClient` pattern by composing their own typed wrapper alongside `AuthClient` + `ProcessorClient` using the same `Dialer`. The `*Dialer` API stays minimal (no API changes for 19.1).

### 3.2 NEW: `ProcessingRequest`/`ProcessingResponse` JSON codec (ADR-0170)

Per parent §3.2 + the public surface sketched at §1 item 5. Filter-local at `internal/filter/http/extproc/json.go`. Uses `protojson` (`google.golang.org/protobuf/encoding/protojson`) which is already in the dependency tree via `go-control-plane`. Public surface (private to the `extproc` package):

```go
// marshalProcessingRequest serializes a ProcessingRequest to JSON bytes.
func marshalProcessingRequest(req *extprocv3.ProcessingRequest) ([]byte, error)

// unmarshalProcessingResponse deserializes JSON bytes into a ProcessingResponse.
func unmarshalProcessingResponse(data []byte) (*extprocv3.ProcessingResponse, error)
```

**`protojson` MarshalOptions** (the IMPL settles the exact options):
- `UseProtoNames: true` (use proto field names — snake_case — NOT lowerCamelCase per the proto3 JSON canonical mapping; IMPL verifies against reference Envoy at §19.P8 IMPL closure).
- `EmitUnpopulated: false` (omit zero-valued fields).
- `UseEnumNumbers: false` (enum-as-string per parent §5.P8 hypothesis).

**`protojson` UnmarshalOptions**:
- `DiscardUnknown: true` (forward-compat for proto extensions added in future Envoy versions).

The codec is invoked from `check.go`'s http_service-mode path: `marshalProcessingRequest` produces the POST body; `unmarshalProcessingResponse` parses the response body; the rest of the per-stage dispatch is mode-agnostic (the converged `applyProcessingResponse` value path).

**Filter-local-vs-shared-package disposition.** The codec is intentionally NOT generalized into `internal/grpcclient/` or a new `internal/jsoncodec/` at 19.1 — there is no second consumer in-tree, and the codec is structurally tied to the ext_proc proto-binding. Per the phase-18.1 ADR-0159 (b)-disposition rationale, generalization is reconsidered at the THIRD consumer trigger. ADR-0170 anchors.

### 3.3 NEW: Symmetric `EncoderFilterCallbacks` extension (ADR-0174)

Per parent §5.P12 REFUTED at SPEC time. `EncoderFilterCallbacks` at master tip (`internal/filter/http/callbacks.go:160-184`) carries ONLY `ContinueEncoding` + `EncodeHeaders/Data/Trailers` + `OverwriteBody`; the 6 socket/TLS/listener accessors land at 19.1 IMPL via ADR-0174:

```go
// EncoderFilterCallbacks gains 6 new methods at 19.1 IMPL per ADR-0174 — the
// symmetric extension of ADR-0165's DecoderFilterCallbacks methods. Required
// for response_attributes population at the response_headers stage.
type EncoderFilterCallbacks interface {
    // ... existing 5 methods (ContinueEncoding, EncodeHeaders/Data/Trailers, OverwriteBody) ...

    // NEW per ADR-0174 — mirror ADR-0165's 6 decode-side accessors.
    DownstreamRemoteAddr() net.Addr
    DownstreamLocalAddr() net.Addr
    DownstreamTLSServerName() string
    DownstreamTLSPeerCertDER() []byte
    DownstreamProtocol() string
    ListenerPrincipal() string
}
```

**Seeding discipline mirrors ADR-0165** (chain.go `tlsPrincipals`/`SetTLSPrincipals` pattern):
- The 6 chain fields seeded at HCM dispatch (H1 `connection.go:dispatchRequest` + H2 `h2dispatch.go:WriteH2`) ALREADY exist per ADR-0165's plumbing — they are SET ONCE at chain build time BEFORE either `RunDecodeHeaders` or `RunEncodeHeaders` dispatch.
- The NEW `*encoderCB` reader methods (added at 19.1 IMPL) return the SAME chain fields verbatim — no new chain plumbing required, only new accessor methods on the encoder-side callbacks struct.
- No race introduced — the chain fields are SET-once at HCM dispatch BEFORE any dispatch path runs; the encoder-side reads happen after that SET completes; ADR-0071's chain-ownership invariant continues to apply.

**Scope (ADR-0174 §Decision draft):** add the 6 reader methods to the `EncoderFilterCallbacks` interface; implement them on `*encoderCB` (the encoder-side struct that wraps the chain pointer); add Group N unit tests asserting the read-back path; no new chain plumbing primitives (the 6 chain fields already exist). NO `DownstreamPrincipal()` extension to the encoder-side callbacks — `DownstreamPrincipal` is decode-side-specific in the ADR-0144 framing (the "decode-side discovers the principal candidates at dispatch" pattern is one-direction); the IMPL adds a 7th method if `response_attributes` allowlist requires it (decided at IMPL — current hypothesis is the 6 ADR-0165 methods suffice). LoC estimate: +30-50 LoC on `callbacks.go` (interface extension) + ~30-50 LoC on the `*encoderCB` implementation + ~80-120 LoC unit tests. ADR-0174 anchors.

**Cross-phase reuse intent.** ADR-0174's encode-side accessor extension is generally useful for any future filter participating on the encode side — current ext_proc is the sole consumer at 19.1, but any future response-mutating filter or response-stage-attribute-emitting filter reuses the same accessor surface.

### 3.4 Framework reuses — ADR-0158 Dialer + ADR-0165 6 callbacks + ADR-0166 plaintext h2c + phase-09 async-resume

Phase 19.1 demonstrates the load-bearing reusability of FOUR prior framework primitives (the FIFTH, ADR-0128 decode-side body-buffering, lands at 19.2):

- **Phase-18.2 ADR-0158 `internal/grpcclient/Dialer`** — REUSED via `NewProcessorClient(*Dialer, ...)`. FIRST cross-phase consumer of ADR-0158 outside phase-18.2 itself.
- **Phase-18.2 ADR-0165 6 new `DecoderFilterCallbacks` methods** — REUSED for `request_attributes` envelope population at the request_headers stage. The attribute set: `source.address` (`DownstreamRemoteAddr()`), `destination.address` (`DownstreamLocalAddr()`), `connection.tls_version` / `connection.subject_local_certificate` (`DownstreamTLSServerName()` + `DownstreamTLSPeerCertDER()`), `request.protocol` (`DownstreamProtocol()`), `connection.principal` (`ListenerPrincipal()`). Plus `source.principal` from `DownstreamPrincipal()` (ADR-0144). 19.1 IMPL pins the exact attribute-name allowlist mapping against reference Envoy v1.37.2's CEL attribute registry per parent §5.P4-class IMPL scrape.
- **Phase-18.2 ADR-0166 cluster-manager plaintext h2c upstream relaxation** — REUSED for the fixture-0022 `c_ext_proc` test cluster (plaintext h2c upstream).
- **Phase-09 async-resume primitive** — REUSED for per-stage parked dispatch (each outbound stage call parks decode or encode goroutine on a resume channel).

NO ADR anchors the reuses themselves (they are reuses, not deltas); ADR-0167 (the phase-19 layout ADR) cites all four reuses + their cross-phase-consumer framing.

### 3.5 No filter-chain ordering surgery

Per §2.7. ext_proc registers as one more extension-registry entry; the HCM filter-chain iteration protocol, the per-route TPFC resolution, and the async-resume primitive are all consumed as-is.

---

## 4. Deny-path wire shape (multi-stage ImmediateResponse + error dispositions)

Per parent SPEC §5.P2 RATIFIED + §6 amendment 2. The mode-agnostic deny-path `SendLocalReply` mechanism is REUSED unchanged from prior §9 filters (ADR-0085); ext_proc's multi-stage capability extends WHEN deny can fire (not how). On an `ImmediateResponse` arrival at the request_headers stage OR the response_headers stage:

- **status** — `ImmediateResponse.status.code` (`*type.v3.HttpStatus`; PGV-required). NOT fixed by the filter — comes from the processor.
- **headers** — `ImmediateResponse.headers` is `*HeaderMutation` (per parent §5.P2 — distinct from phase-18.2's `DeniedHttpResponse.headers` which is plain `[]HeaderValueOption`). The IMPL applies SET-or-REMOVE via the `HeaderMutation.set_headers` + `remove_headers` fields. `mutation_rules` per-header gating applies (parent §5.P3) — rejected mutations dropped + `spurious_msgs_received` counter increment. `content-length` synthesized by ADR-0085.
- **body** — `ImmediateResponse.body` (`[]byte`) reproduced verbatim. The proto doc states "sent using the text/plain content type, or encoded in the grpc-message header" — the IMPL applies the content-type discipline based on `content-type: application/grpc` request-header sniff:
  - non-gRPC downstream: body emitted with `content-type: text/plain` (default; unless overridden by `headers.set_headers[content-type]`).
  - gRPC downstream (detected by request `content-type: application/grpc`): body encoded into the `grpc-message` response header (per the proto doc); `content-type: application/grpc` on the response.
- **grpc_status** — `*GrpcStatus` carrying a `status uint32`. When non-nil AND the downstream is gRPC (detected by request `content-type: application/grpc`), the IMPL adds a `grpc-status: <status>` response trailer. When non-nil but downstream is non-gRPC, IGNORED (the field has no HTTP-only translation).
- **details** — `string`. Maps to `response_code_details` — DEFERRED for MVP per §2.8 (envoy-go HCM does not surface `response_code_details`).

On an **error** disposition (processor unreachable, message_timeout exceeded, gRPC transport failure, malformed response):

- `failure_mode_allow: false` (proto default) → `SendLocalReply(500, "", {})` per the proto's `failure_mode_allow` doc ("the filter will fail. Specifically, if the response headers have not yet been delivered, then it will return a 500 error downstream. If they have been delivered, then instead the HTTP stream to the downstream client will be reset"). The `streams_failed` counter increments. The "response headers have not yet been delivered" branch fires for errors at the request_headers / request_body stages; the "stream reset" branch fires for errors at the response stages (after response headers delivered) — the IMPL applies a stream-reset via the framework's existing reset primitive (`f.dcb.SendLocalReply(0, ...)` with status 0 is the framework's stream-reset signal per phase-04, OR an alternate framework primitive — IMPL settles). 19.1 fixture exercises the request_headers-stage error branch (the response-stage-error branch is not in the 19.1 scenario matrix; reserved for IMPL-time verification or 19.2 fixture extension).
- `failure_mode_allow: true` → the request is allowed through (`HeaderContinue`); the `failure_mode_allowed` AND `streams_failed` counters BOTH increment.

The over-stream-timeout `message_timeout` exceeded behavior (the per-message timer expires without a response) is classified as an error per the above. The `disable_immediate_response=true` configuration silently drops ImmediateResponse messages from the processor (treated as a protocol violation; `spurious_msgs_received` increments; stream continues without the local reply emission).

ADR-0167 anchors the deny-path mechanism; ADR-0172 anchors the multi-stage `ImmediateResponse` handling + the gRPC-downstream-detection discipline + the grpc_status translation.

---

## 5. Per-route discipline — 5th canonical REUSE (NO new canonical) + SHARED-stats

Per parent SPEC §5.P6 RATIFIED + §6 amendment 6 + ADR-0173. `ExtProcPerRoute` carries one PGV-required oneof `override` with two arms:

- **`disabled` (bool, PGV `const: true`)** — `ExtProcPerRoute{disabled: true}` wholly deactivates the filter on the route: `DecodeHeaders` returns `HeaderContinue` immediately, no processor call, no counter increments, request forwards as-is. envoy-go PARSE-REJECTs `disabled: false` (the PGV `const: true` constraint — same wrinkle as phase-18 ext_authz).
- **`overrides` (`*ExtProcOverrides`, PGV `required` within the arm)** — a NARROWER per-route override carrying **7 fields** at `go-control-plane v1.32.4`: `processing_mode` (#1, per-route ProcessingMode override; MVP-CONSUMED), `async_mode` (#2, `[#not-implemented-hide:]` — silent-ignore), `request_attributes` (#3, `[#not-implemented-hide:]` — silent-ignore; distinct from the TOP-LEVEL `ExternalProcessor.request_attributes` #5 which IS MVP-consumed for the listener-level envelope), `response_attributes` (#4, `[#not-implemented-hide:]` — silent-ignore; distinct from the TOP-LEVEL `ExternalProcessor.response_attributes` #6), `grpc_service` (#5, per-route service override; MVP-CONSUMED — useful for routing different paths to different processor backends), `metadata_options` (#6, dynamic-metadata family — DEFERRED), `grpc_initial_metadata` (#7, initial-metadata family — DEFERRED). The BRAINSTORM §2.6 hypothesis is RATIFIED per parent §5.P6 / §6 amendment 6.

This maps cleanly onto **ADR-0125's existing 5th canonical** (disabled-bool arm + a NARROWER override sub-message arm in a oneof; the pattern phase-13 buffer + phase-14 compressor + phase-18 ext_authz already use). **Phase 19 lands NO ADR-0125 amendment paragraph** — the SECOND CONSECUTIVE §9 family-row after phase 18 to REUSE rather than extend. ADR-0173 records the explicit 5th-canonical-REUSE classification (the absence of a §(xiv) amendment is itself a recorded decision — strengthens the ADR-0125 roster-not-monotonic lesson).

**Per-route stats SHARED with listener-level** (per parent §6 amendment 4 + ADR-0173): the per-route override adjusts `processing_mode`/`grpc_service` but still hits the same external processor — it spawns no new stateful policy-evaluation surface. SHARED-stats; MIRRORS phase-12/13/14/17/18 SHARED-stats discipline; DIVERGES from phase-11/15/16 INDEPENDENT-stats. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request via the existing TPFC resolution machinery; the IMPL caches-on-first-use per parent §5.P7 (the per-route config resolved at `DecodeHeaders` time stays in effect for the entire bidi-stream's lifetime, even across `ClearRouteCache` invocations).

The 9-counter stat surface (per parent §5.P4 hypothesis; RATIFIED-PENDING-IMPL-TIME) lands at 19.1 unconditionally — all 9 counters register at `New()` time even when the listener-level config never fires the corresponding code path (mirrors phase-18.2 ADR-0163 `disabled`-counter STRUCTURALLY-UNREACHABLE discipline for scrape-stability).

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

```go
package extproc

const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"

// New is the HTTPFilterFactory the boot registry invokes for each
// type-URL match per ADR-0072.
func New(cfg *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.HTTPFilter, error)
```

Plus the NEW `internal/grpcclient/` extension — public surface UNCHANGED at the package level (still `Dialer` + factory functions); new type `ProcessorClient` + `NewProcessorClient` + `(*ProcessorClient).Process` + `(*ProcessorClient).Close` added per §3.1.

### 6.2 `compiledConfig` shape

The `compiledConfig` struct (in `extproc.go`) carries the per-listener parsed configuration. Field-final at 19.1 per ADR-0168 §Decision (mirrors phase-18.1 ADR-0157 §Decision); 19.2 amends only to lift body-mode PARSE-REJECT (the struct gains NO new fields — body-mode-specific config lives in the closure captures inside `processFn`).

```go
type compiledConfig struct {
    // Transport — mutually exclusive: exactly one of these is non-nil.
    grpcClient *grpcclient.ProcessorClient // when grpc_service is set
    httpClient *httpProcessorClient        // when http_service is set
    httpServiceHeadersOnly bool            // true when http_service mode (body PARSE-REJECT per proto constraint)

    // Processing mode + mode-override.
    processingMode      *resolvedProcessingMode // header-modes only in 19.1; body-modes activate at 19.2
    allowModeOverride   bool
    allowedOverrideModes []*resolvedProcessingMode // empty list = no restriction

    // Error posture.
    failureModeAllow         bool
    messageTimeout           time.Duration
    maxMessageTimeout        time.Duration // 0 = override disabled
    disableImmediateResponse bool

    // Mutation discipline.
    mutationRules  *resolvedMutationRules // pre-compiled allow/deny matchers for header-mutation safety
    forwardRules   *resolvedForwardRules  // pre-compiled allowed/disallowed header matchers for forwarding

    // Attribute envelopes.
    requestAttributes  []string // CEL attribute names for request_headers stage
    responseAttributes []string // CEL attribute names for response_headers stage

    // Route-cache discipline.
    routeCacheAction extprocv3.ExternalProcessor_RouteCacheAction // translates disable_clear_route_cache=true to RETAIN

    // Stat surface — 9 counters per parent §5.P4 hypothesis.
    stats *filterStats // SHARED with per-route per ADR-0173

    // Stat-prefix segment if set.
    statPrefix string
}
```

`resolvedProcessingMode` is a parsed-and-validated form of `ProcessingMode` (with DEFAULT translated per parent §5.P9: DEFAULT → SEND for headers; trailer-modes restricted to SKIP; body-modes restricted to NONE in 19.1). `resolvedMutationRules` is a pre-compiled form of `HeaderMutationRules` (with allow/deny matchers ready to apply). `resolvedForwardRules` is a pre-compiled form of `HeaderForwardingRules` (similar). The IMPL settles the exact `resolved*` struct shapes.

### 6.3 `DecodeHeaders` body

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus {
    // 1. Resolve per-route → cache on filter state.
    pr := f.resolvePerRoute()
    if pr.disabled {
        return Continue
    }

    // 2. Check request_header_mode for the resolved processing_mode.
    rc := pr.effectiveProcessingMode(f.cc.processingMode)
    if rc.RequestHeaderMode == SKIP {
        return Continue
    }

    // 3. Build the request_headers ProcessingRequest (populates attributes
    //    via the ADR-0165 6 decoder-callback accessors + ADR-0144 DownstreamPrincipal).
    pr := buildRequestHeadersProcessingRequest(f, headers, endStream, f.cc.requestAttributes)

    // 4. Open the bidi-stream (gRPC mode) OR prepare the HTTP-mode per-stage POST.
    //    Stream lifecycle: opened at this stage's outbound dispatch; closed at OnDestroy
    //    OR at final response stage OR at ImmediateResponse arrival.
    f.openProcessorStream() // gRPC: open the Process bidi-stream; HTTP: noop (per-stage POST)

    // 5. Fire async send + recv; park the decode goroutine on the resume channel.
    f.dispatchStage(stageRequestHeaders, pr)
    return StopIteration
}
```

The stage dispatch is asynchronous — a goroutine performs the `Send`+`Recv` (gRPC) or the `http.Client.Do` (HTTP) and signals `f.dcb.ContinueDecoding()` on completion. The recv path applies `CommonResponse.header_mutation` to the headers via `f.dcb.RequestHeaders().Set/Add/Del` (or analog framework primitives) BEFORE the `ContinueDecoding` signal. ImmediateResponse triggers `f.dcb.SendLocalReply(status, body, headers)` instead of resuming. Mode_override (when applicable per parent §5.P1) updates `f.activeProcessingMode` for SUBSEQUENT stages.

### 6.4 `EncodeHeaders` body

```go
func (f *filter) EncodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus {
    // 1. Skip if filter disabled-per-route (already checked at DecodeHeaders;
    //    cache the per-route state on filter for encode-side reuse).
    if f.perRoute.disabled {
        return Continue
    }

    // 2. Check response_header_mode for the effective processing_mode (may have
    //    been updated mid-stream via mode_override).
    if f.activeProcessingMode.ResponseHeaderMode == SKIP {
        return Continue
    }

    // 3. Build the response_headers ProcessingRequest (populates response_attributes
    //    via the NEW ADR-0174 6 encoder-callback accessors).
    pr := buildResponseHeadersProcessingRequest(f, headers, endStream, f.cc.responseAttributes)

    // 4. Fire async send + recv on the EXISTING bidi-stream (gRPC mode — same Process
    //    stream that decode-side opened) OR a fresh HTTP POST (http_service mode).
    //    The stream is shared decode→encode across the request lifecycle.
    f.dispatchStage(stageResponseHeaders, pr)
    return StopIteration
}
```

The encode-side dispatch goroutine parks on the encode resume channel (mirrors decode-side; phase-09 async-resume primitive reuse on the encode side — FIRST §9 row to exercise encode-side async-resume parking). The recv path applies `CommonResponse.header_mutation` via `f.ecb.ResponseHeaders().Set/Add/Del`. ImmediateResponse at response_headers triggers `f.ecb.SendLocalReply(...)` — the framework's `SendLocalReply` enters the encode chain at `filter[len-1]` per ADR-0075, which means the immediate-response emission supersedes any in-flight encode chain participation. ADR-0167 codifies the encode-side participation.

### 6.5 `buildCompiledConfig` — the `grpc_service` / `http_service` dispatch

```go
func buildCompiledConfig(raw *extprocv3.ExternalProcessor, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
    cc := &compiledConfig{}

    // 1. Mutual-exclusion check on grpc_service vs http_service (parent §5.P1).
    grpcSet := raw.GetGrpcService() != nil
    httpSet := raw.GetHttpService() != nil
    if grpcSet == httpSet {
        return nil, errors.New("ext_proc: exactly one of grpc_service or http_service must be set")
    }

    // 2. Build the transport client.
    if grpcSet {
        // Validate the EnvoyGrpc arm; GoogleGrpc PARSE-REJECT per ADR-0157 §Decision AMENDMENT.
        cc.grpcClient, err = buildGRPCProcessorClient(raw.GetGrpcService(), ctx)
    } else {
        cc.httpClient, err = buildHTTPProcessorClient(raw.GetHttpService())
        cc.httpServiceHeadersOnly = true
    }
    if err != nil { return nil, err }

    // 3. Validate and resolve processing_mode (parent §5.P9).
    //    - 19.1 PARSE-REJECT: body-mode != NONE; trailer-mode != SKIP; STREAMED-only flags.
    //    - http_service mode: body-mode != NONE → PARSE-REJECT per the proto constraint.
    cc.processingMode, err = resolveProcessingMode(raw.GetProcessingMode(), cc.httpServiceHeadersOnly)
    if err != nil { return nil, err }

    // 4. Validate error-posture fields (message_timeout default 200ms; max_message_timeout default 0).
    cc.failureModeAllow = raw.GetFailureModeAllow()
    cc.messageTimeout, err = durationpbToGoOrDefault(raw.GetMessageTimeout(), 200*time.Millisecond)
    cc.maxMessageTimeout, _ = durationpbToGoOrDefault(raw.GetMaxMessageTimeout(), 0)
    cc.disableImmediateResponse = raw.GetDisableImmediateResponse()

    // 5. STREAMED-only flag PARSE-REJECT (parent §5.P10).
    if raw.GetObservabilityMode() { return nil, errors.New("ext_proc: observability_mode not supported (STREAMED-only flag, out of envelope)") }
    if raw.GetSendBodyWithoutWaitingForHeaderResponse() { return nil, errors.New(...) }
    if dt := raw.GetDeferredCloseTimeout(); dt != nil && dt.AsDuration() > 0 { return nil, errors.New(...) }

    // 6. route_cache_action mutual-exclusion with disable_clear_route_cache (parent §5.P5).
    rcaSet := raw.GetRouteCacheAction() != 0
    dcrcSet := raw.GetDisableClearRouteCache()
    if rcaSet && dcrcSet { return nil, errors.New("ext_proc: only one of route_cache_action or disable_clear_route_cache can be set") }
    if dcrcSet { cc.routeCacheAction = extprocv3.ExternalProcessor_RETAIN } else { cc.routeCacheAction = raw.GetRouteCacheAction() }

    // 7. Resolve mutation_rules + forward_rules.
    cc.mutationRules, err = resolveMutationRules(raw.GetMutationRules())
    cc.forwardRules, err = resolveForwardRules(raw.GetForwardRules())

    // 8. Allowed override modes.
    cc.allowModeOverride = raw.GetAllowModeOverride()
    cc.allowedOverrideModes, err = resolveAllowedOverrideModes(raw.GetAllowedOverrideModes())

    // 9. Attribute envelopes.
    cc.requestAttributes = raw.GetRequestAttributes()
    cc.responseAttributes = raw.GetResponseAttributes()

    // 10. Stat-prefix + filterStats allocation.
    cc.statPrefix = raw.GetStatPrefix()
    cc.stats = newFilterStats(ctx, cc.statPrefix)

    return cc, nil
}
```

`resolveProcessingMode` is the load-bearing validation routine: it (a) PARSE-REJECTs `request_body_mode`/`response_body_mode` ∉ {NONE} (in 19.1); (b) PARSE-REJECTs `request_trailer_mode`/`response_trailer_mode` ∉ {SKIP} (permanently); (c) translates `DEFAULT` → SEND for header-modes (parent §5.P9); (d) PARSE-REJECTs body-modes when `httpServiceHeadersOnly` is true per the proto constraint.

`buildGRPCProcessorClient` mirrors phase-18.2 `buildGRPCCheckFn` (per ADR-0158 §Decision (iv) + ADR-0169):
1. PARSE-REJECT `GoogleGrpc` arm (`ADR-0157` §Decision AMENDMENT inherited).
2. SILENT-IGNORE `initial_metadata` + `retry_policy`.
3. Look up cluster via `ctx.ClusterManager.Get(clusterName)` → PARSE-REJECT if not found OR `UseH2()==false`.
4. Construct `*grpcclient.ProcessorClient` via `grpcclient.NewProcessorClient(dialer, clusterName, messageTimeout)`.

`buildHTTPProcessorClient` is a 19.1-new type (filter-local at `extproc/check.go`):
1. Validate `HttpService.server_uri.uri` set + non-empty.
2. Construct an `*http.Client{Timeout: hs.server_uri.timeout}`.
3. Capture `path_prefix` for per-stage URL construction.

### 6.6 `buildAttributeEnvelope` — `request_attributes` / `response_attributes` population

Per parent §5.P1 + the proto's CEL attribute-name allowlist. The IMPL populates the `ProcessingRequest.attributes` field (`map[string]*structpb.Struct`) at each header stage according to the configured allowlist + the per-stream state captured via the ADR-0165 (decode-side) + ADR-0174 (encode-side) accessor surfaces.

**Hypothesized attribute-name mapping** (RATIFIED-PENDING-IMPL-TIME per parent §5.P4-class):

| CEL attribute name | Source (decode) | Source (encode) |
|---|---|---|
| `source.address` | `f.dcb.DownstreamRemoteAddr()` | `f.ecb.DownstreamRemoteAddr()` |
| `destination.address` | `f.dcb.DownstreamLocalAddr()` | `f.ecb.DownstreamLocalAddr()` |
| `connection.requested_server_name` | `f.dcb.DownstreamTLSServerName()` | `f.ecb.DownstreamTLSServerName()` |
| `connection.subject_local_certificate` | (derived from listener cert + ADR-0144) | (same) |
| `request.protocol` | `f.dcb.DownstreamProtocol()` | `f.ecb.DownstreamProtocol()` |
| `connection.principal` | `f.dcb.ListenerPrincipal()` | `f.ecb.ListenerPrincipal()` |
| `source.principal` | `f.dcb.DownstreamPrincipal()[0]` (first or "") | (DECODE-only — `DownstreamPrincipal` is not on encoder-side per §3.3) |

19.1 IMPL settles the exact attribute-name → accessor mapping by scraping a reference Envoy v1.37.2 `CheckRequest`-analog `ProcessingRequest` with various attribute allowlists. If response-side `source.principal` is required, the IMPL extends ADR-0174's 6 methods to 7 (decided at IMPL).

The codec wraps each attribute value in a `*structpb.Value` per the protobuf CEL attribute encoding.

### 6.7 `applyProcessingResponse` — the per-stage response application

Mode-agnostic (gRPC + HTTP-mode converge):

```go
func (f *filter) applyProcessingResponse(stage stage, resp *extprocv3.ProcessingResponse) (action, error) {
    // 1. ImmediateResponse — overrides everything (unless disable_immediate_response).
    if ir := resp.GetImmediateResponse(); ir != nil {
        if f.cc.disableImmediateResponse {
            f.cc.stats.spuriousMsgsReceived.Inc()
            return actContinue, nil // silent-drop the ImmediateResponse
        }
        return f.emitImmediateResponse(ir), nil // SendLocalReply via decoder or encoder cb
    }

    // 2. override_message_timeout (if present, other fields ignored).
    if ot := resp.GetOverrideMessageTimeout(); ot != nil {
        f.handleOverrideMessageTimeout(ot)
        return actContinueButStillWaiting, nil // timer reset; stage still in-flight
    }

    // 3. mode_override (if present + allowed) — applied for SUBSEQUENT stages.
    if mo := resp.GetModeOverride(); mo != nil {
        if isHeaderResponseStage(stage) && f.cc.allowModeOverride && f.isModeAllowed(mo) {
            f.activeProcessingMode = applyModeOverride(f.activeProcessingMode, mo)
        } else {
            // mode_override on non-header response, OR not allowed → silently ignored per parent §5.P1
            // (NOT classified as spurious_msgs_received per the proto doc)
        }
    }

    // 4. CommonResponse for the matching stage.
    var cr *extprocv3.CommonResponse
    switch stage {
    case stageRequestHeaders:
        if hr := resp.GetRequestHeaders(); hr != nil { cr = hr.GetResponse() }
    case stageResponseHeaders:
        if hr := resp.GetResponseHeaders(); hr != nil { cr = hr.GetResponse() }
    // body-stage cases land at 19.2
    }
    if cr == nil {
        // Stage-mismatch (e.g., server returned response_headers for our request_headers send).
        // Classify as spurious + dispError.
        f.cc.stats.spuriousMsgsReceived.Inc()
        return actError, errStageMismatch
    }

    // 5. Apply header_mutation per direction + mutation_rules.
    if hm := cr.GetHeaderMutation(); hm != nil {
        f.applyHeaderMutation(stage, hm)
    }

    // 6. clear_route_cache + route_cache_action precedence (request_headers stage only per parent §5.P5).
    if stage == stageRequestHeaders {
        if f.shouldClearRouteCache(cr.GetClearRouteCache()) {
            f.dcb.ClearRouteCache()
        }
    }

    // 7. status — CONTINUE is default; CONTINUE_AND_REPLACE lands at 19.2 (PARSE-REJECT here in 19.1).
    if cr.GetStatus() == extprocv3.CommonResponse_CONTINUE_AND_REPLACE {
        f.cc.stats.spuriousMsgsReceived.Inc()
        return actError, errors.New("ext_proc: CONTINUE_AND_REPLACE not supported in 19.1 (lands at 19.2 with body-mode activation)")
    }

    return actContinue, nil
}
```

`f.applyHeaderMutation` iterates `HeaderMutation.set_headers` + `remove_headers`; for each mutation, calls `f.mutationRules.IsAllowed(name)` (per parent §5.P3 per-header gating); if allowed, applies via `f.dcb.RequestHeaders().Set/Add/Del` (or `f.ecb.ResponseHeaders().Set/Add/Del`); if rejected, drops the mutation AND sets a per-stage flag so `spurious_msgs_received` increments ONCE per stage with any rejection.

`f.emitImmediateResponse` constructs the local-reply per §4. It calls `f.dcb.SendLocalReply` (decode stages) or `f.ecb.SendLocalReply` (encode stages) — both framework primitives produce the same wire shape per ADR-0085. Per parent §5.P2 the gRPC-downstream detection sniffs `f.requestContentType == "application/grpc"`; if so, `body` → `grpc-message` header + `grpc_status` → `grpc-status` trailer.

### 6.8 `f.openProcessorStream` + `f.dispatchStage` (gRPC mode)

```go
func (f *filter) openProcessorStream() {
    f.streamCtx, f.streamCancel = context.WithCancel(f.parentCtx)
    f.stream, _ = f.cc.grpcClient.Process(f.streamCtx) // error caught at first Send
}

func (f *filter) dispatchStage(stage stage, req *extprocv3.ProcessingRequest) {
    go func() {
        sendCtx, sendCancel := context.WithTimeout(f.streamCtx, f.cc.messageTimeout)
        defer sendCancel()
        if err := f.stream.Send(req); err != nil {
            f.cc.stats.streamsFailed.Inc()
            f.completeStage(stage, nil, err)
            return
        }
        f.cc.stats.streamMsgsSent.Inc()
        recvCtx, recvCancel := context.WithTimeout(f.streamCtx, f.cc.messageTimeout)
        defer recvCancel()
        resp, err := f.stream.Recv()
        if err != nil {
            f.cc.stats.streamsFailed.Inc()
            f.completeStage(stage, nil, err)
            return
        }
        f.cc.stats.streamMsgsReceived.Inc()
        f.completeStage(stage, resp, nil)
    }()
}
```

`f.completeStage` calls `applyProcessingResponse` synchronously inside the goroutine, then signals `f.dcb.ContinueDecoding()` (or `f.ecb.ContinueEncoding()`) on the resume channel. The dispatch goroutine is parked on `StopIteration` until the signal fires.

`OnDestroy` (the filter-lifetime cancel hook) invokes `f.streamCancel()` + `f.stream.CloseSend()` — the in-flight `Send`/`Recv` calls return promptly with the cancellation error. ADR-0167 codifies the bidi-stream lifecycle.

### 6.9 File layout — multi-file split

Per the phase-18.1+18.2 multi-file split precedent:

- `extproc.go` (~400-600 LoC) — filter type + factory + decode AND encode methods + `filterStats` + `compiledConfig` + per-route helper.
- `check.go` (~600-900 LoC) — the per-stage dispatcher; `buildGRPCProcessorClient` + `buildHTTPProcessorClient`; `applyProcessingResponse`; `applyHeaderMutation`; `emitImmediateResponse`; the `mode_override` validator; the `route_cache_action` translator; the failure-mode handler.
- `attributes.go` (~250-400 LoC) — `buildAttributeEnvelope` + the attribute-name → accessor map + helpers (mirrors the phase-18.2 `attributes.go` extension shape).
- `processor.go` (~250-400 LoC) — the state-machine model (stage enum, per-direction ProcessingMode state, dispatchStage, completeStage, the resume-channel discipline, OnDestroy cancel + CloseSend).
- `json.go` (~150-250 LoC) — `marshalProcessingRequest` + `unmarshalProcessingResponse` + `protojson` MarshalOptions/UnmarshalOptions setup.
- `extproc_test.go` (~1500-2500 LoC) — unit tests across Groups 1-N (mirrors phase-18.x test grouping).
- `fuzz_test.go` (~100 LoC) — the 24th fuzzer `FuzzExtProcConfigParse`.
- `doc.go` (~50 LoC) — package overview.

Plus the NEW file `internal/grpcclient/processor_client.go` (~150-250 LoC) for ADR-0169. Plus the modifications to `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` for ADR-0174 (the 6 new methods on `EncoderFilterCallbacks` + their `*encoderCB` implementations — anticipated ~50-100 LoC; no chain plumbing changes since the chain fields already exist per ADR-0165).

### 6.10 Compile-time invariants

- `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` — decode side.
- `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` — encode side.
- `var _ envoyhttp.HTTPFilter = (*filter)(nil)` — the both-sides shape.

---

## 7. Differential fixture `0022-http-ext-proc-grpc`

Per parent SPEC §2 + this SPEC §1. **6-8 scenarios** — covering BOTH service modes + the two header stages + the per-route discipline + the failure-mode posture. The fixture exercises HEADERS STAGES ONLY (body-stage scenarios land at 19.2 in fixture `0023-http-ext-proc-body`).

### 7.1 Per-request matrix

| # | Scenario | Listener / Per-route | Processor script | Expected disposition | Counter delta assertion |
|---|---|---|---|---|---|
| 1 | gRPC allow + header set | l_test_a / default | `ProcessingResponse{request_headers: HeadersResponse{response: CommonResponse{header_mutation{set_headers}}}}` → server then sends second response for response_headers stage with `CommonResponse{}` | 200 backend echo; injected header arrives upstream | `streams_started=1` + `stream_msgs_sent=2` + `stream_msgs_received=2` + `streams_closed=1` |
| 2 | gRPC immediate_response at request_headers | l_test_a / `/scenario2` | `ProcessingResponse{immediate_response: ImmediateResponse{status: 403, body: "denied", headers: HeaderMutation{set_headers: [...]}}}` | 403 + body byte-exact + injected headers | `streams_started=1` + `stream_msgs_sent=1` + `stream_msgs_received=1` + `streams_closed=1` |
| 3 | gRPC response_headers mutation | l_test_a / `/scenario3` | request_headers response: `CommonResponse{}` (continue); response_headers response: `CommonResponse{header_mutation{set_headers}}` | 200 backend echo; injected header arrives downstream | `stream_msgs_sent=2` + `stream_msgs_received=2` |
| 4 | gRPC mode_override mid-stream | l_test_a / `/scenario4` | request_headers response: `CommonResponse{} + mode_override{response_header_mode: SKIP}` (assuming SKIP in `allowed_override_modes`) | 200 backend echo; response_headers stage SKIPPED (no response_headers ProcessingRequest sent) | `stream_msgs_sent=1` + `stream_msgs_received=1` |
| 5 | gRPC failure_mode_allow | l_test_b / default | processor unreachable; `failure_mode_allow: true` | 200 backend echo (unprocessed); request forwards upstream | `streams_failed=1` + `failure_mode_allowed=1` |
| 6 | HTTP allow (headers-only) | l_test_c / default | HTTP 200 + JSON ProcessingResponse with header_mutation | 200 backend echo; injected header arrives upstream | `stream_msgs_sent=2` + `stream_msgs_received=2` (one per header stage) |
| 7 | per-route disabled | l_test_a / `/disabled` | (no processor call made) | 200 backend echo | **NO `ext_proc` counter increments** (disabled discipline mirrors phase-18.2 ext_authz disabled) |
| 8 (opt) | per-route processing_mode override | l_test_a / `/override` | per-route `overrides{processing_mode{request_header_mode: SKIP, response_header_mode: SEND}}`; response_headers response with header mutation | 200 backend echo; request_headers stage skipped; response_headers stage runs | `stream_msgs_sent=1` + `stream_msgs_received=1` |

The 19.1 fixture exercises BOTH transports (gRPC for scenarios 1-5+7-8; HTTP for scenario 6) but ONLY the header stages. Three-listener topology mirrors phase-18.x (l_test_a for the main matrix; l_test_b for failure_mode_allow with `failure_mode_allow: true`; l_test_c for the HTTP-service-mode listener). The 19.2 fixture `0023-http-ext-proc-body` adds body-stage scenarios (request_body BUFFERED replace, response_body BUFFERED replace, CONTINUE_AND_REPLACE, multi-stage immediate_response at response_body, etc.).

### 7.2 Topology + test-helper

`envoy.yaml` + `envoy-go.yaml` each wire three HCM listeners (l_test_a/b/c) with the ext_proc filter (gRPC-mode for a/b; HTTP-mode for c) + a router, an echo upstream cluster (REUSES `test/helpers/echobackend/`), and the ext_proc `grpc_service.envoy_grpc.cluster_name: "ext_proc_grpc"` pointing at the processor cluster — which references a NEW test-helper: an **in-process bidi-stream gRPC `ExternalProcessor.Process` server** under `test/helpers/extprocgrpc/`. The helper is spawned-per-fixture (mirrors `test/helpers/extauthzgrpc/` lifecycle; extends from unary→bidi). Plaintext h2c for the processor cluster (REUSES ADR-0166 plaintext h2c upstream relaxation; mirrors fixture-0021 `c_authz_grpc`). The HTTP-service test-helper for scenario 6 may reuse `test/helpers/extauthzhttp/` shape OR introduce a thin NEW `test/helpers/extprochttp/` per IMPL settle.

**No listener-side TLS in the 19.1 fixture.** Like phase-18.2, the listener-side TLS scenarios are deferred to a future TLS-listener-extension fixture; the `request_attributes` envelope's TLS-attribute population (via the ADR-0165 6 + ADR-0174 6 accessors) is unit-tested with mocked TLS state.

The driver in `inputs/driver.go` exercises the 6-8 scenarios; the harness asserts response status + body byte-equivalence on allow AND deny paths, `/stats/prometheus` counter-delta equivalence on the reachable counters, backend-arrival header assertions (allow-path upstream injection + scenario 1 + 8 mutations), downstream-arrival header assertions (scenario 3 + 6 mutations), and processor-server received-ProcessingRequest content assertions (per-stage discriminator).

### 7.3 24th fuzzer — `FuzzExtProcConfigParse`

NEW `FuzzExtProcConfigParse` at `internal/filter/http/extproc/fuzz_test.go`:

```go
func FuzzExtProcConfigParse(f *testing.F) {
    f.Add([]byte{0x00}) // empty proto
    // ... seed with valid ExternalProcessor encodings: gRPC mode, HTTP mode, various per-route, various processing_mode, various error-posture
    f.Fuzz(func(t *testing.T, data []byte) {
        var raw extprocv3.ExternalProcessor
        if err := proto.Unmarshal(data, &raw); err != nil { return }
        // Wrap in an *anypb.Any for the factory.
        any, err := anypb.New(&raw)
        if err != nil { return }
        _, err = New(any, mockFactoryCtx)
        // Asserts: factory returns (non-nil filter, nil err) OR (nil filter, non-nil err); never panics.
    })
}
```

Corpus seeds: 8-12 encoded `ExternalProcessor` variants covering both service modes, various processing_mode combinations (incl. PARSE-REJECT-triggering body/trailer/STREAMED variants), various error-posture configurations (with/without `failure_mode_allow`, `disable_immediate_response`, various timeouts), various per-route configs (disabled, overrides with processing_mode, overrides with grpc_service). 30s/seed under the ADR-0018 budget. ext_proc's response-mapping fuzzer (`FuzzProcessingResponseMapping`) is 19.2's deliverable since the body-stage CommonResponse mappings are part of its scope.

### 7.4 `test/helpers/extprocgrpc/` package

The FIRST in-tree bidi-stream gRPC server in envoy-go's test tree (the phase-18.2 `test/helpers/extauthzgrpc/` is unary). Surface:

```go
package extprocgrpc

// Server is an in-process envoy.service.ext_proc.v3.ExternalProcessor.Process server.
// Spawn-per-fixture lifecycle. Plaintext h2c on a randomly-bound port.
type Server struct {
    addr    string
    grpcSrv *grpc.Server
    scripts map[string][]*extprocv3.ProcessingResponse // keyed by discriminator (e.g., :path); ordered per stage
    mu      sync.RWMutex
}

// New starts a Server bound to 127.0.0.1:0 (ephemeral).
func New(t testing.TB) *Server

// Addr returns the host:port the Server is bound to.
func (s *Server) Addr() string

// Script registers a sequence of ProcessingResponse values for the given
// discriminator. Each Recv from the stream advances the script counter
// for that discriminator; the next response in the sequence is sent.
// The discriminator is derived from the first ProcessingRequest received
// on the stream (typically request_headers stage with a specific :path).
func (s *Server) Script(discriminator string, responses ...*extprocv3.ProcessingResponse)

// Stop graceful-shutdowns the gRPC server.
func (s *Server) Stop()
```

The discriminator is the request's `:path` extracted from `req.GetRequestHeaders().GetHeaders()` (so scenario URLs `/scenario1`, `/scenario2`, … each get their own scripted response sequence). Per-fixture cleanup via `t.Cleanup(s.Stop)`. ~200-300 LoC production + ~120-180 LoC tests. The IMPL settles whether to support a non-`:path` discriminator (e.g., context-extension value) — current SPEC scope is `:path`-keyed.

---

## 8. Deferred items (19.1 slice; per parent SPEC §4.4 + §8)

For future-phase consideration (none are blockers for closing row 19.1; all auditable in the ADR-0040 deferral trail). 19.1's deferred slice + the 19.2 carry-forward + the future-phase items:

1. **Body-stage activation** (`request_body_mode`/`response_body_mode = BUFFERED`) — DEFERRED to 19.2. PARSE-REJECT in 19.1.
2. **STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes** — DEFERRED permanently (out of envelope per Q2; requires a future streaming-body framework phase).
3. **Trailer modes** (request_trailer_mode + response_trailer_mode anything other than SKIP) — DEFERRED permanently (out of envelope per Q2; couples to a framework-wide trailer-pass-through primitive blocked since phase-04 HTTP/1.1).
4. **`observability_mode`** — DEFERRED permanently (STREAMED-only flag; out of envelope per Q2; PARSE-REJECT).
5. **`send_body_without_waiting_for_header_response`** — DEFERRED permanently (STREAMED-only flag; PARSE-REJECT).
6. **`deferred_close_timeout`** — DEFERRED permanently (STREAMED-only flag; PARSE-REJECT for non-zero).
7. **`metadata_options` (forwarding_namespaces + receiving_namespaces)** — DEFERRED (dynamic-metadata family — blocked at phases 16+17+18 forward-pointers).
8. **`filter_metadata`** — DEFERRED (same dynamic-metadata coupling).
9. **`ProcessingResponse.dynamic_metadata`** emit + `ProcessingRequest.metadata_context` — DEFERRED (dynamic-metadata family; the empty-proto-message shape stays for forward-compat).
10. **`CommonResponse.dynamic_metadata` and `CommonResponse.trailers`** — DEFERRED (both proto-flagged `[#not-implemented-hide:]` or dynamic-metadata family).
11. **`HttpHeaders.attributes`** (deprecated + `[#not-implemented-hide:]`) — silent-ignore (the proto-doc says it's deprecated and not implemented; envoy-go honors the convention).
12. **`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}`** — DEFERRED per ADR-0173 (silent-ignore the three `[#not-implemented-hide:]` fields `async_mode` + `request_attributes` + `response_attributes`; defer the dynamic-metadata + initial-metadata families). The top-level `ExternalProcessor.request_attributes`/`response_attributes` (#5 + #6) are MVP-CONSUMED for the listener-level attribute envelope — distinct from these silent-ignored per-route override fields.
13. **`core.GrpcService.GoogleGrpc` arm** — envoy-go-strict EXCLUSION (PARSE-REJECT — inherited from ADR-0157 §Decision AMENDMENT; envoy-go uses Go gRPC directly).
14. **`core.GrpcService.{initial_metadata, retry_policy}`** — SILENT-IGNORED for MVP (gRPC client retry + initial-metadata follow-ups).
15. **`response_code_details` emission** (the `ImmediateResponse.details` field) — DEFERRED (envoy-go HCM does not surface response_code_details; joint divergence with phase-16/17/18 forward-pointers).
16. **xDS-CDS-driven processor-cluster reconfig** — DEFERRED (envoy-go has no xDS-CDS yet).
17. **TLS-fronted processor-cluster fixture coverage** — DEFERRED (fixture 0022 uses plaintext h2c; mirrors phase-18.2 fixture 0021 disposition).
18. **`request_attributes`/`response_attributes` CEL-attribute-name allowlist exact roster** — RATIFIED-PENDING-IMPL-TIME at 19.1 IMPL (parent §5.P4-class; the IMPL settles the exact mapping from `request_attributes` list entries to envoy-go callback accessor calls against reference Envoy v1.37.2's CEL attribute registry).

---

## 9. Cross-references against phase-18 + phase-17 deferred-items lists — forward-pointer pickup

- **Phase-18 dynamic-metadata family** (`metadata_context_namespaces`, `typed_metadata_context_namespaces`, `dynamic_metadata_from_headers`, `CheckResponse.dynamic_metadata`): NO PICKUP — ext_proc's `metadata_options` + `filter_metadata` + `CommonResponse.dynamic_metadata`-style emit (§8 items 7-10) EXTEND the dynamic-metadata deferred-cluster; ext_proc is now the FOURTH §9 filter (after rbac + jwt_authn + ext_authz) blocked on the dynamic-metadata family — strengthening the case for a dedicated dynamic-metadata-family phase.
- **Phase-18 `allowed_client_headers_on_success`** (decode-side feasibility): NOT APPLICABLE — ext_proc operates encode-side natively (response_headers stage runs encode-side); the decode-side-only feasibility constraint phase-18 ext_authz faced does not apply.
- **Phase-18 GoogleGrpc arm**: CONTINUED DEFERRAL (ext_proc PARSE-REJECTs the GoogleGrpc arm under ADR-0157 §Decision AMENDMENT).
- **Phase-18 gRPC retry-policy customization**: CONTINUED DEFERRAL.
- **Phase-18 `response_code_details` framework primitive** (joint forward-pointer with rbac + jwt_authn): NO PICKUP at MVP — ext_proc's `ImmediateResponse.details` mapping ADDS to the joint-closure forward-pointer (now blocked across phases 16+17+18+19). Strengthens the case for a dedicated phase.
- **Phase-18.2 ADR-0165 callback-surface extension on `EncoderFilterCallbacks`**: **CLOSED at 19.1** — ADR-0174 extends the 6 accessors to the encoder-side callbacks (the encode-side-asymmetry concern phase-18.2 left open).
- **Phase-13 ADR-0128 encode-side mirror** (not previously deferred — anticipated as a future framework lift): **NOT CLOSED at 19.1** — lands at 19.2 as ADR-0175.

**Forward-pointer net change for phase 19.1**: 1 closure (ADR-0174 closes the encode-side callback symmetry concern from phase-18.2). 19.1 adds 18 new deferred items (§8 above) + EXTENDS the dynamic-metadata family + EXTENDS the streaming-body-framework deferred-cluster (new at phase 19 — items 2 + 4-6) + ADDS to the `response_code_details` joint-closure forward-pointer.

---

## 10. ADR anchor map (19.1 subset; full 10-ADR map in parent SPEC §7)

The 19.1-landing ADRs. Per the ADR-0044 ADR-on-impl convention: ADR-0167..ADR-0174 §Context drafts are anchored at the parent SPEC commit; §Decision + §Consequences LAND at each ADR's Lands-in-Task per ADR-0044. ADR-0176 lands IN FULL at the parent SPEC commit.

| ADR | Subject (19.1 portion) | Lands-in-Task (hypothesis) |
|---|---|---|
| **ADR-0167** | `internal/filter/http/extproc/` package shape — single-token directory + BOTH-DECODE-AND-ENCODE `HTTPFilter` value + 9-base-counter `filterStats` registered unconditionally at `New()` time + boot-registration alphabetical between `extauthz` and `fault` + multi-stage `SendLocalReply` mechanism (request_headers + response_headers in 19.1; body stages at 19.2) + the TWELFTH §9 row framing + FIRST-cross-phase-consumer-of-ADR-0158/ADR-0165/ADR-0166 framing | Task 2 (factory + filterStats) |
| **ADR-0168** | `compiledConfig` shape + the `grpc_service`-vs-`http_service` mutually-exclusive top-level field dispatch + the http_service proto-constraint (PARSE-REJECT body when http_service is set) + the body-mode 19.1 PARSE-REJECT + trailer-mode PARSE-REJECT + STREAMED-only flag PARSE-REJECT + consumed-vs-deferred field discipline + the error-posture fields | Task 2 (compiledConfig + buildCompiledConfig) |
| **ADR-0169** | `*ProcessorClient` bidi-stream wrapper EXTENDING `internal/grpcclient/Dialer` + cross-phase-reuse intent | Task 3 (`internal/grpcclient/processor_client.go`) |
| **ADR-0170** | `ProcessingRequest`/`ProcessingResponse` JSON codec for http_service mode + filter-local-vs-shared-package disposition + protojson defaults RATIFIED-PENDING-IMPL-TIME | Task 4 (`extproc/json.go`) |
| **ADR-0171** | ProcessingMode + mode-override state-machine discipline (HEADER-MODE PORTION) — DEFAULT translates to SEND for headers / SKIP for trailers; mode_override on header-response paths only; `allow_mode_override` + `allowed_override_modes` validation; max_message_timeout bounding override_message_timeout; STREAMED-only flags PARSE-REJECT | Task 5 (`processor.go` state machine) |
| **ADR-0172** | CommonResponse mutation + ImmediateResponse multi-stage deny (HEADER_MUTATION + IMMEDIATE_RESPONSE-AT-HEADERS PORTION) — `header_mutation` set/remove per direction; `mutation_rules` per-header gating; `clear_route_cache` + `route_cache_action` precedence (request-headers stage only); ImmediateResponse with `*HeaderMutation` + grpc_status content-type sniff | Task 6 (`check.go` applyHeaderMutation + emitImmediateResponse) |
| **ADR-0173** | Per-route 5th-canonical REUSE classification (explicit no-new-canonical; NO ADR-0125 §(xiv) amendment) + SHARED-stats + ExtProcOverrides narrower-override surface + 9-counter stat surface RATIFIED-PENDING-IMPL-TIME | Task 7 (per-route resolution + stat registration) |
| **ADR-0174** | Symmetric `EncoderFilterCallbacks` extension — 6 new methods (DownstreamRemoteAddr/LocalAddr/TLSServerName/TLSPeerCertDER/Protocol + ListenerPrincipal) + chain-field reuse (no new chain plumbing — ADR-0165's chain fields are SET-once at HCM dispatch BEFORE either decode or encode dispatch) + cross-phase-reuse intent | Task 4 (PRE-REQUISITE — must land before Task 5 since attributes.go consumes the new accessors). Anchored as Task 4 in PLAN — IMPL lands `callbacks.go` extension + encoder-side reader methods + unit tests BEFORE the filter consumes them |

**ADR-0044 escape-valve** held in reserve for ~0-2 impl-time-unanticipated ADRs. The SPEC-time in-session SCRAPE closure of §19.P11 + §19.P12 (REFUTED → ADR-0174 fires at SPEC time; ADR-0175 fires at 19.2 SPEC time) REMOVES the most-likely escape-valve surfaces. Other possible 19.1 IMPL-time surfaces: (i) the bidi-stream cancellation discipline interacting with HCM's stream lifecycle (a new dispatch pattern for envoy-go — phase-18.2's unary RPC was the precedent; the bidi adds the half-close lifecycle layer); (ii) the JSON codec wire-shape divergence if §19.P8 IMPL scrape surfaces deltas from `protojson` defaults; (iii) the route_cache_action vs disable_clear_route_cache precedence interaction if §19.P5 + §19.P7 IMPL scrapes surface deltas; (iv) the `request_attributes` allowlist-to-accessor mapping if reference Envoy's attribute-name registry differs from the hypothesized mapping. **NO new framework primitives** beyond ADR-0169 + ADR-0170 + ADR-0174. **NO new SN-flattening rule** unless IMPL-time confirmation refutes the §5.P4 SN2-reuse hypothesis. **NO ADR-0125 amendment.**

**Next-free ADR after phase 19.1** = ADR-0175 (reserved for 19.2). Phase-19.1 IMPL has NO RATIFIED-PENDING pin closures except for the empirical-scrape pins (§19.P4 stat surface, §19.P7 per-route re-resolution, §19.P8 JSON codec wire shape) per parent SPEC §5; these close at specific 19.1 IMPL tasks (the fixture-harness empirical scrape task for §19.P4 + §19.P8; a specific scenario in fixture 0022 for §19.P7).

---

## 11. Empirical-pin block reference (parent §5 carries all 13 pins)

The 13 empirical pins (§19.P1..§19.P13) live in the parent master SPEC §5. They span both sub-phases; resolving them ONCE in the parent is the discipline. Of the 13, two are REFUTED at SPEC time (§19.P11 → ADR-0175 fires at 19.2; §19.P12 → ADR-0174 fires at 19.1); three are RATIFIED-PENDING-IMPL-TIME (§19.P4 stat surface, §19.P7 per-route re-resolution, §19.P8 JSON codec wire shape — all close at 19.1 IMPL fixture-harness empirical scrape tasks per phase-16 §10 lesson (c) + phase-18.2 §11.P13 precedent). The remaining 8 are RATIFIED or RATIFIED-AND-REFINED in-session.

**19.1-specific in-IMPL closures (4 pins):** §19.P4 (9-counter stat surface) + §19.P7 (cache-on-first-use per-route discipline) + §19.P8 (JSON codec wire shape vs protojson defaults) + §19.P13 (fuzzer corpus extends clean — though P13 is RATIFIED at SPEC time + reconfirmed at IMPL by running the suite). The IMPL scrapes against reference Envoy v1.37.2 fixture-harness assertions close these pins.

---

## 12. Deferred decisions (the planner / implementer settles these)

1. **`test/helpers/extprocgrpc/` exact API** — discriminator key (the §7.4 sketch proposes `:path` keyed; the IMPL may choose another key like the request's `x-test-scenario` header or a per-stream context-extension value if more flexible).
2. **`*grpc.ClientConn` close-on-process-exit discipline** — whether to register an `os.Exit`-time cleanup hook in the `internal/grpcclient/` package, or just let GC handle it for MVP. Per ADR-0158 §Decision (vi) + the phase-18.2 precedent, MVP leaks-on-exit; the IMPL may add a hook if cheap.
3. **`request_attributes`/`response_attributes` exact accessor map** — the §6.6 table is the hypothesis; IMPL scrapes reference Envoy v1.37.2 to settle the exact CEL-attribute-name → callback-accessor mapping. Possible refinements: `connection.tls_version` might come from a derived value rather than directly from `DownstreamTLSServerName()`; `source.principal` might require iterating the slice rather than just the first element.
4. **`*HeaderMutation.set_headers` `HeaderValueOption.append_action` enum mapping** — same 4-arm dispatch as phase-18.2 ext_authz `OkHttpResponse.headers` (`APPEND_IF_EXISTS_OR_ADD`, `OVERWRITE_IF_EXISTS_OR_ADD`, `OVERWRITE_IF_EXISTS`, `ADD_IF_ABSENT`). IMPL settles per the phase-10 header_mutation precedent + the phase-18.2 settle.
5. **Reset-vs-local-reply discipline on error after response-headers-delivered** — the proto `failure_mode_allow` doc states "if they have been delivered, then instead the HTTP stream to the downstream client will be reset". The IMPL settles whether `f.dcb.SendLocalReply(0, ...)` is the framework's stream-reset signal OR whether a new framework primitive is required. Anticipated: the existing primitive suffices (NO new ADR fires).
6. **`override_message_timeout` timer-reset implementation** — whether to use a single `time.AfterFunc`-backed timer with `Reset()` calls, or a `context.WithTimeout` cancel-and-rebuild. IMPL settles.
7. **CONTINUE_AND_REPLACE PARSE-REJECT vs silent-classify-as-error in 19.1** — the §6.7 sketch classifies CONTINUE_AND_REPLACE as `spurious_msgs_received` + dispError because the body-stage activation is 19.2's scope. IMPL settles whether this is correct (silently classify the malformed-for-19.1 message) OR whether to surface the operator-facing error differently. Anticipated: classify as error per the §6.7 sketch.
8. **JSON codec error handling** — on `unmarshalProcessingResponse` failure (e.g., malformed JSON from the processor), classify as `streams_failed` + dispError. IMPL settles whether to drop the stage's response silently (rare; for forward-compat with future proto extensions when `DiscardUnknown: true` doesn't cover) or fail-loud.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at 19.1 phase-done)

1. **§13.1 — `## HTTP filter chain` → NEW `### envoy.filters.http.ext_proc` subsection.** Covers: the `services` mutually-exclusive dispatch (grpc_service + http_service); the body-mode PARSE-REJECT in 19.1 (body-stage modes — see phase 19.2 forward-pointer); the trailer-mode PARSE-REJECT permanently; the STREAMED-only flag PARSE-REJECT permanently; the ProcessingMode state-machine + mode-override discipline + `allow_mode_override` + `allowed_override_modes`; the CommonResponse header_mutation per direction per stage; the multi-stage ImmediateResponse (request_headers + response_headers stages — body-stage variants at 19.2); the `clear_route_cache` + `route_cache_action` precedence + request-headers-stage-only-application; the `failure_mode_allow` / `message_timeout` / `max_message_timeout` / `disable_immediate_response` error posture; the gRPC-downstream-detection via content-type sniff for grpc_status translation; the per-route 5th-canonical REUSE + SHARED-stats; the request_attributes + response_attributes envelope discipline.
2. **§13.2 — `## Stat-name mapping` → 77 → ~86-name table extension.** Adds 9 new rows for the ext_proc counters under `http.<HCM_stat_prefix>.ext_proc.*` per parent §5.P4 hypothesis (RATIFIED-PENDING-IMPL-TIME — if the IMPL scrape diverges, the row count adjusts at IMPL).
3. **§13.3 — `## Equivalence Matrix` → NEW row for fixture `0022-http-ext-proc-grpc`** with byte-exact body + status + verbatim header assertions + counter-delta equivalence + ProcessingRequest-content assertion (the processor server received the expected attributes envelope).
4. **§13.4 — NEW `### Phase 19.1 forward-pointer notes` subsection** covering the §8 deferral list (18 items — body-stage activation forward-pointer to 19.2; permanent deferrals for STREAMED/trailer modes + dynamic-metadata family + GoogleGrpc + retry-policy + response_code_details). 19.1 closes 1 phase-18.2 forward-pointer (encode-side callback symmetry via ADR-0174).
5. **§13.5 — `## HTTPFilterCallbacks` extension AMENDMENT (per ADR-0174).** Adds a paragraph documenting the 6 new methods landed on `EncoderFilterCallbacks`: `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. The chain-side seeding discipline is UNCHANGED from ADR-0165 (the chain fields are SET-once at HCM dispatch BEFORE either decode or encode dispatch); the AMENDMENT adds only the encode-side reader methods. Cross-references ADR-0174 for the cross-phase reuse intent. Mirrors the phase-18.2 §13.5 AMENDMENT pattern.
6. **§13.6 — `## Per-route canonical patterns cross-reference` — NO new entry.** Phase 19.1 REUSES the existing 5th canonical; ADR-0173 records the explicit no-amendment 5th-canonical-REUSE decision (the absence of a §(xiv) amendment is itself a recorded decision — strengthens the ADR-0125 roster-not-monotonic lesson).
7. **§13.7 — `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella EXTENSION.** Adds a paragraph documenting the NEW `*ProcessorClient` bidi-stream wrapper (ADR-0169) alongside the existing unary `*AuthClient`; same package, same `Dialer` integration, NO `Dialer` API changes. Cross-references the public surface at §3.1. NOTE: this is an extension to the EXISTING phase-18.2 umbrella, NOT a new top-level umbrella — bidi-stream is a usage pattern over the same dial layer, not a new framework family.
8. **§13.8 — `### JSON codec note` (lighter-touch reference).** A thin paragraph under the §13.7 umbrella (NOT a new top-level umbrella per the phase-18.1 ADR-0159 (b)-disposition rationale) documenting the filter-local `ProcessingRequest`/`ProcessingResponse` JSON codec (ADR-0170) at `internal/filter/http/extproc/json.go`. Forward-pointer: generalization to `internal/jsoncodec/` at the second-consumer trigger.

---

## 14. Testing strategy

### 14.1 Unit tests

- `internal/grpcclient/processor_client_test.go` — Group 1 NEW: `NewProcessorClient` happy path (plaintext cluster); PARSE-REJECT for unknown cluster; PARSE-REJECT for `UseH2: false` cluster. Group 2 NEW: `(*ProcessorClient).Process` bidi-stream lifecycle — open stream → Send → Recv → CloseSend → stream close; mid-stream cancel via parent ctx cancellation; per-message timeout propagation. Group 3 NEW: `(*ProcessorClient).Close` idempotency.
- `internal/filter/http/extproc/extproc_test.go` — Group 1: factory parse paths (mutual-exclusion, body-mode PARSE-REJECT, trailer-mode PARSE-REJECT, STREAMED-only flag PARSE-REJECT, GoogleGrpc PARSE-REJECT, route_cache_action mutual-exclusion). Group 2: `compiledConfig` field values post-parse. Group 3: `buildGRPCProcessorClient` + `buildHTTPProcessorClient` construction paths. Group 4: `applyHeaderMutation` + `mutationRules` per-header gating. Group 5: `applyProcessingResponse` per-stage dispatch (request_headers / response_headers; non-applicable stages classified as spurious). Group 6: `emitImmediateResponse` for both decode and encode stages + grpc_status content-type sniff translation. Group 7: ProcessingMode resolution + mode_override re-eval discipline + `allow_mode_override` + `allowed_override_modes` validation. Group 8: per-route 5th-canonical REUSE + cache-on-first-use + 9-counter SHARED-stats. Group 9: error-posture (`failure_mode_allow` true/false; `message_timeout` exceeded; `disable_immediate_response`). Group 10: `OnDestroy` cancels in-flight bidi-stream + CloseSend. Group 11: attribute envelope builder (mocked TLS state in *filter; assert ProcessingRequest.attributes content).
- `internal/filter/http/callbacks_test.go` extensions — Group 12 NEW (per ADR-0174): the 6 new `EncoderFilterCallbacks` methods seed-and-read paths (one test per new method asserting `Set(seed) → ecb.Get() == seed`) plus 6 nil/empty fall-through tests. Mirrors the `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed` template.
- Existing Group 1-N from phases 09-18 are UNCHANGED.

### 14.2 Race detector + lint

`go test -race ./internal/grpcclient/... ./internal/filter/http/extproc/...` + repo-wide race clean. The bidi-stream + multi-stage per-stream cancellation path is a likely race-detector exercise surface. Specific concerns the race tests must cover:

- **`OnDestroy`-driven cancel during in-flight `ProcessStream.Send` or `Recv`.** Same primitive 18.2 ratified for `AuthClient.Check`: `mu`/`done` guard + `context.WithCancel`; the per-stage context threads into the gRPC bidi stream; cancellation surfaces promptly. The 19.1 IMPL adds `TestOnDestroy_CancelsInFlightProcessorStream`.
- **Concurrent encode-stage dispatch alongside decode-stage dispatch on the SAME bidi stream.** This is a NEW concern for envoy-go — phase 18.2's unary `Check` had no inter-stage concurrency. ext_proc's request_headers stage dispatch goroutine completes BEFORE the response_headers stage dispatch goroutine starts (the framework dispatches decode then encode sequentially per HTTP transaction); but if the framework ever dispatched concurrently (e.g., for a future encode-side-only filter co-existing on the same chain), the shared `*ProcessStream` would race. The IMPL settles whether to add a per-stream mutex around `Send`/`Recv` calls OR rely on the framework's sequential decode→encode dispatch guarantee. Race-test exercises sequential decode→encode + asserts no race.
- **Mode_override mid-stream race with mode-evaluation at subsequent stages.** The `f.activeProcessingMode` field is mutated on the recv path of the request_headers stage's response goroutine; it is READ on the dispatch path of the response_headers stage (after the framework dispatch hands off encode-side control). The IMPL settles whether atomic load/store or a mutex is needed; given the sequential decode→encode dispatch, a non-atomic READ-after-WRITE may be safe BUT the IMPL race-tests confirm.
- **bidi-stream `Send` blocking on the goroutine that called `Recv` returning** — the gRPC library documents that concurrent `Send` and `Recv` are safe on a single `ClientStream` but neither is safe with another caller of the same direction; the IMPL ensures only one goroutine calls `Send` and only one calls `Recv` per stage's dispatch.

### 14.3 Fuzzer

24th fuzzer `FuzzExtProcConfigParse` per §7.3. Existing 23 fuzzers re-run clean.

### 14.4 h2spec + differential

h2spec 53/53 PASS at the ADR-0051 pin (NO H2 wire-shape change between 18.2 and 19.1; ext_proc uses gRPC over H2 to the upstream processor cluster, not to the downstream client; the downstream client's H1/H2 behavior is unchanged). 23 differential fixtures green at 19.1 phase-done (0000–0022; 0022 NEW; 0000–0021 carry-forward).

### 14.5 Six-gate checklist (A/B/C/D/E/F per BOOTSTRAP_PROMPT.md §7.5)

- **Gate A** (build + vet + lint): green; the `internal/grpcclient/` extension compiles clean; the `internal/filter/http/extproc/` new package compiles clean; the `internal/filter/http/callbacks.go` extension (ADR-0174) compiles clean.
- **Gate B** (race tests): green; `go test -race ./internal/grpcclient/... ./internal/filter/http/extproc/... ./internal/filter/http/...` + repo-wide.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin.
- **Gate D** (fuzzers): 24 fuzzers green at 30s each.
- **Gate E** (differential): 23/23 fixtures green (0000–0022).
- **Gate F** (BEHAVIOR_CONTRACT): the §13 edit bundle landed; `tools/check_behavior_contract.sh` (or analog) green.

---

## 15. Acceptance checklist (for the reviewer)

The 19.1 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts:

1. **`internal/filter/http/extproc/` package per ADR-0167:** `internal/filter/http/extproc/{extproc.go, check.go, attributes.go, processor.go, json.go, extproc_test.go, fuzz_test.go, doc.go}` landed; `TypeURL` constant + `New` factory per the cors/.../extauthz precedent; alphabetical boot-registration insertion between `extauthz` and `fault`; both `StreamDecoderFilter` AND `StreamEncoderFilter` compile-time assertions; 9-counter `filterStats` unconditionally registered at `New()` time.
2. **`compiledConfig` shape + dual-mode dispatch per ADR-0168:** `grpc_service` + `http_service` mutual-exclusion at parse time (both-set OR neither-set → PARSE-REJECT); body-mode (request/response_body_mode != NONE) PARSE-REJECT in 19.1; trailer-mode (request/response_trailer_mode != SKIP) PARSE-REJECT permanently; STREAMED-only flag (`observability_mode`, `send_body_without_waiting_for_header_response`, non-zero `deferred_close_timeout`) PARSE-REJECT; HTTP-service body-mode PARSE-REJECT (per proto constraint); GoogleGrpc arm PARSE-REJECT (per ADR-0157 §Decision AMENDMENT inherited); initial_metadata + retry_policy SILENT-IGNORED.
3. **`*ProcessorClient` bidi-stream wrapper per ADR-0169:** `internal/grpcclient/processor_client.go` landed; reuses existing `*Dialer` (NO `Dialer` API changes); per-message timeout via `context.WithTimeout`; one `*grpc.ClientConn` per (cluster, compiledConfig) pair; leaks-on-exit MVP; cross-phase reuse forward-pointer in ADR-0169 §Decision.
4. **JSON codec per ADR-0170:** `internal/filter/http/extproc/json.go` landed; `marshalProcessingRequest` + `unmarshalProcessingResponse`; `protojson` MarshalOptions `UseProtoNames: true` + `EmitUnpopulated: false` + `UseEnumNumbers: false`; UnmarshalOptions `DiscardUnknown: true`; filter-local (NOT generalized to `internal/jsoncodec/`); §19.P8 RATIFIED at the fixture-harness empirical scrape task (the per-stage POST byte-equivalence vs reference Envoy v1.37.2).
5. **ProcessingMode state machine per ADR-0171 (header-mode portion):** `DEFAULT` translates to SEND for header-modes + SKIP for trailer-modes at parse time; mode_override on header-response paths only (silently ignored on body/trailer-response paths); `allow_mode_override` + `allowed_override_modes` validation; `max_message_timeout >= 1ms` gates `override_message_timeout` API enablement; `override_message_timeout` range check [1ms, max_message_timeout]; STREAMED-only flag PARSE-REJECT.
6. **CommonResponse mutation + ImmediateResponse multi-stage per ADR-0172 (header-mutation + immediate_response-at-headers portion):** `header_mutation` set/remove applied per direction per stage; `mutation_rules` per-header gating (allowed mutations apply; rejected mutations dropped + `spurious_msgs_received` counter increment ONCE per stage with any rejection); `clear_route_cache` + `route_cache_action` precedence (mutual exclusion at parse time; request-headers stage only); ImmediateResponse with `*HeaderMutation` SET/REMOVE; gRPC-downstream-detection via content-type sniff for grpc_status translation; CONTINUE_AND_REPLACE classified as `spurious_msgs_received` + dispError in 19.1.
7. **Per-route 5th-canonical REUSE + SHARED-stats per ADR-0173:** `ExtProcPerRoute.override` oneof PGV-required; `disabled` PGV `const: true`; `overrides` `*ExtProcOverrides` (7 fields); MVP-CONSUMED `processing_mode` + `grpc_service` per-route overrides; `async_mode` + `request_attributes` + `response_attributes` + `metadata_options` + `grpc_initial_metadata` silent-ignored/deferred (the per-route `request_attributes`/`response_attributes` at #3/#4 are flagged `[#not-implemented-hide:]` — distinct from the top-level `ExternalProcessor.request_attributes`/`response_attributes` at #5/#6 which ARE MVP-consumed); SHARED-stats with listener-level; cache-on-first-use per parent §5.P7; NO ADR-0125 §(xiv) amendment paragraph.
8. **Symmetric `EncoderFilterCallbacks` extension per ADR-0174:** the 6 new methods landed on `EncoderFilterCallbacks` + the `*encoderCB` reader implementations + Group 12 unit tests; NO new chain plumbing (ADR-0165 chain fields are SET-once at HCM dispatch BEFORE either decode or encode dispatch); race-detector verification clean across `./internal/filter/http/... ./internal/filter/hcm/...`.
9. **Empirical pins:** parent §5 13 pins all CLOSED at the 19.1 phase-done commit — RATIFIED/RATIFIED-AND-REFINED at SPEC time for 9 pins; REFUTED at SPEC time for 2 pins (§19.P11 + §19.P12 — ADR-0174 + ADR-0175 fire); RATIFIED-PENDING-IMPL-TIME at SPEC time for 3 pins (§19.P4 stat surface + §19.P7 per-route re-resolution + §19.P8 JSON codec wire shape — all closed RATIFIED at 19.1 IMPL fixture-harness scrape). 19.1 IMPL has zero RATIFIED-PENDING pins remaining.
10. **Differential fixture `0022-http-ext-proc-grpc` per §7:** 6-8 scenarios across BOTH service modes + the two header stages + per-route discipline + failure_mode_allow + mode_override + immediate_response; three-listener topology (l_test_a/b/c — a for main matrix; b for failure_mode_allow; c for HTTP-service-mode); byte-exact body + status on allow + deny paths; cross-side counter-delta equivalence on the reachable counters; processor-server received-ProcessingRequest assertions including the attribute envelope content; 1 NEW test-helper `test/helpers/extprocgrpc/` (FIRST in-tree bidi-stream gRPC test-helper).
11. **24th fuzzer per §7.3:** `FuzzExtProcConfigParse` at `internal/filter/http/extproc/fuzz_test.go`; 30s ADR-0018 budget; existing 23 fuzzers re-run clean.
12. **BEHAVIOR_CONTRACT.md populated** per Gate F: §13.1 NEW ext_proc subsection with body-mode forward-pointer to 19.2; §13.2 stat-table 77 → ~86 names; §13.3 NEW row for 0022; §13.4 NEW `### Phase 19.1 forward-pointer notes`; §13.5 `## HTTPFilterCallbacks` AMENDMENT for ADR-0174; §13.7 phase-18.2 gRPC client umbrella EXTENSION for ADR-0169 bidi-stream wrapper; §13.8 JSON codec lighter-touch note.
13. **DECISIONS.md populated** per ADR-on-impl convention: ADR-0167..ADR-0174 §Decision + §Consequences landed at their respective Lands-in-Tasks (§Context already at the parent SPEC commit); ADR-0176 §Decision + §Consequences already at the parent SPEC commit; ADR-0175 §Context anchored at the parent SPEC commit but §Decision + §Consequences land at 19.2. NO new ADR numbers consumed in 19.1 IMPL (ADR-0177 remains next-free; if an unanticipated ADR DOES land, it is ADR-0177 — but the SPEC-time scrape closure of §19.P11/§19.P12 reduces this likelihood substantially).
14. **ROADMAP.md** row `19.1` flips `in-progress → done` at the 19.1 phase-done commit. Rows `19` (parent) + `19.2` are UNCHANGED at this commit (parent stays `in-progress`; `19.2` stays `planned`). Per parent SPEC §8 — the parent row 19 closes AT THE SAME commit as 19.2's phase-done, NOT at 19.1's.
15. **All six phase-done gates green** at the 19.1 phase-done commit: build/vet/lint clean; race-test clean repo-wide; h2spec 53/53 PASS; 24 fuzzers green at 30s; 23 differential fixtures green (0000–0022); BEHAVIOR_CONTRACT.md populated.
16. **No master mutation outside the 19.1 squash-merge commit** — all work landed on the 19.1 worktree branches per ADR-0005 §Decision 4 + project memory `feedback_git_worktrees.md`; master tip advances only at the squash-merge commit + SHA-fill follow-up.

End of phase 19.1 SPEC.
