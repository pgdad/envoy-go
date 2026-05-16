# Phase 19.1 — HTTP filter `envoy.filters.http.ext_proc` (filter scaffold + headers stages + bidi-stream primitive + encode-side callback symmetry) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.ext_proc` in **headers-stages-only mode** — the foundational half of the phase-19 ADR-0045 split begun at the phase-19 parent SPEC commit — by shipping the NEW `internal/filter/http/extproc/` package (TWELFTH §9 HTTP filter; FIRST §9 row to participate on BOTH `StreamDecoderFilter` AND `StreamEncoderFilter` in a single filter package), the NEW `*ProcessorClient` bidi-stream wrapper at `internal/grpcclient/processor_client.go` EXTENDING the phase-18.2 `*Dialer` (ADR-0169 — envoy-go's FIRST bidi-stream gRPC infrastructure; cross-phase-reusable), the NEW symmetric `EncoderFilterCallbacks` extension at `internal/filter/http/callbacks.go` (ADR-0174 — 6 new methods mirroring ADR-0165's decode-side; REFUTED at parent §5.P12 → fires load-bearing; PRE-REQUISITE for the `response_attributes` envelope at the response_headers stage), the NEW filter-local JSON codec at `extproc/json.go` (ADR-0170 — `protojson` for `ProcessingRequest`/`ProcessingResponse` in `http_service` mode), the per-stage state machine + mode-override re-eval (ADR-0171 header-mode portion), the multi-stage `ImmediateResponse` deny path + `CommonResponse.header_mutation` per direction per stage + `mutation_rules` per-header gating + `clear_route_cache` + `route_cache_action` precedence (ADR-0172 header-mode portion), the per-route 5th-canonical REUSE + SHARED-stats + 9-counter filter stat surface (ADR-0173 — SECOND CONSECUTIVE §9 row to REUSE the 5th canonical; NO ADR-0125 amendment paragraph), the differential fixture `0022-http-ext-proc-grpc` (~6-8 headers-only scenarios across BOTH service modes + the per-route discipline + failure_mode_allow + mode_override + immediate_response; three-listener topology l_test_a/b/c), the NEW test-helper `test/helpers/extprocgrpc/` (FIRST in-tree bidi-stream gRPC `ExternalProcessor.Process` server in envoy-go's test tree), and the 24th fuzzer `FuzzExtProcConfigParse` — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis except the documented divergence-windows. **Body-stage activation (`request_body_mode`/`response_body_mode = BUFFERED`) PARSE-REJECTs in 19.1** — that activation + ADR-0175 (encode-side body-buffering primitive) land in 19.2. **19.1's phase-done commit closes ONLY row `19.1`; the parent row `19` stays `in-progress` until 19.2's phase-done per parent SPEC §8 rollup discipline.**

**Architecture:** The 19.1 IMPL extends the existing 07.1 HTTP filter framework with FOUR new ADR-anchored deltas (ADR-0169 bidi-stream wrapper, ADR-0170 JSON codec, ADR-0174 encode-side callback symmetry, ADR-0167+0173 package shape + stat surface) + reuses FIVE existing framework primitives load-bearing (ADR-0158 `internal/grpcclient/Dialer` — FIRST cross-phase consumer outside phase-18.2 itself; ADR-0165 6 `DecoderFilterCallbacks` methods — FIRST cross-phase consumer, populates `request_attributes` envelope at request_headers; ADR-0166 cluster-manager plaintext h2c upstream relaxation — REUSED for the fixture-0022 `c_ext_proc` test cluster; phase-09 async-resume primitive — REUSED on BOTH decode and encode side (FIRST encode-side async-resume consumer); ADR-0085 `SendLocalReply` — REUSED including the FIRST encode-side deny-path emission at the response_headers stage). The NEW `internal/filter/http/extproc/` package follows the phase-18.1+18.2 multi-file split: `doc.go` (~50 LoC) + `extproc.go` (~400-600 LoC — filter type + factory + decode AND encode methods + `filterStats` struct + `compiledConfig` + per-route helper) + `check.go` (~600-900 LoC — `buildGRPCProcessorClient` + `buildHTTPProcessorClient` + `applyProcessingResponse` + `applyHeaderMutation` + `emitImmediateResponse` + `mode_override` validator + `route_cache_action` translator + failure-mode handler) + `attributes.go` (~250-400 LoC — `buildAttributeEnvelope` + attribute-name → accessor map + helpers) + `processor.go` (~250-400 LoC — state-machine model: stage enum, per-direction ProcessingMode state, dispatchStage, completeStage, resume-channel discipline, OnDestroy cancel + CloseSend) + `json.go` (~150-250 LoC — `marshalProcessingRequest` + `unmarshalProcessingResponse` + `protojson` MarshalOptions setup) + `extproc_test.go` (~1500-2500 LoC — Groups 1-11) + `fuzz_test.go` (~100 LoC — 24th fuzzer). The NEW `internal/grpcclient/processor_client.go` (~250-400 LoC) carries the typed bidi-stream wrapper alongside the existing `*AuthClient` (NO new file extracted from `grpcclient.go` per planner-time decision D13). `internal/filter/http/callbacks.go` gains 6 new methods on `EncoderFilterCallbacks` (~50-100 LoC) + `*encoderCB` reader implementations; NO new `chain.go` plumbing (ADR-0165's chain fields are SET-once at HCM dispatch BEFORE either decode or encode dispatch — the new encoder-side readers consume the SAME fields). NEW `test/helpers/extprocgrpc/` (~350-500 LoC) hosts the FIRST in-tree bidi-stream gRPC `ExternalProcessor.Process` server (plaintext h2c on ephemeral port; `:path`-keyed scriptable per-stage `ProcessingResponse` sequences per planner-time decision D1). Differential fixture `0022-http-ext-proc-grpc` (~1000 LoC across envoy.yaml + envoy-go.yaml + expectations.yaml + README.md + inputs/driver.go) wires 8 scenarios over a three-listener topology (l_test_a for main matrix; l_test_b for failure_mode_allow with `failure_mode_allow:true`; l_test_c for HTTP-service-mode). The 9-counter `filterStats` (per parent §5.P4 hypothesis RATIFIED-PENDING-IMPL-TIME — `streams_started` / `stream_msgs_sent` / `stream_msgs_received` / `spurious_msgs_received` / `streams_failed` / `streams_closed` / `failure_mode_allowed` / `override_message_timeout_received` / `override_message_timeout_ignored` under `http.<HCM_stat_prefix>.ext_proc.*`) registers unconditionally at `New()` time; per-route stats SHARED with listener-level per ADR-0173. Cache-on-first-use per-route discipline per parent §5.P7 (the per-route config resolved at `DecodeHeaders` time stays in effect for the entire bidi-stream's lifetime, even across `ClearRouteCache` invocations). Three RATIFIED-PENDING-IMPL-TIME pin closures land at the fixture-harness empirical scrape task (Task 13): §19.P4 (9-counter stat surface name match against reference Envoy v1.37.2), §19.P7 (per-route cache-on-first-use after `ClearRouteCache` mid-stream), §19.P8 (JSON codec wire-shape vs `protojson` defaults — one request/response pair scrape). The phase-19 parent SPEC anchored 10 ADRs at the SPEC commit (ADR-0167..ADR-0175 §Context drafts + ADR-0176 IN FULL); the 19.1 IMPL lands ADR-0167..ADR-0174 §Decision + §Consequences bodies at their respective Tasks per ADR-0044. ADR-0175 lands at 19.2 IMPL (NOT 19.1). ADR-0044 escape-valve held in reserve for ~0-2 IMPL-time-unanticipated ADRs; the SPEC-time in-session scrape closure of §19.P11/§19.P12 (BOTH conditional ADRs REFUTED → fire load-bearing) REMOVES the most-likely escape-valve surfaces — PLAN's strong hypothesis: NO additional ADR fires at 19.1 IMPL (next-free ADR-0177 remains unconsumed at 19.1 phase-done).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/ext_proc/v3` for the filter config + `envoy/service/ext_proc/v3` for `ProcessingRequest`/`ProcessingResponse` + the `ExternalProcessor.Process` bidi-stream RPC stub at `external_processor_grpc.pb.go` + `envoy/config/common/mutation_rules/v3` for `HeaderMutationRules` + `envoy/config/core/v3` for `GrpcService` + `envoy/type/v3` for `HttpStatus`); `google.golang.org/grpc` v1.70.0 (already a DIRECT module dep from phase-18.2 per ADR-0158); `google.golang.org/protobuf/encoding/protojson` for `ProcessingRequest`/`ProcessingResponse` JSON codec in `http_service` mode (already an indirect dep via `go-control-plane`); `google.golang.org/protobuf/types/known/durationpb` for `override_message_timeout` parsing; `google.golang.org/protobuf/types/known/structpb` for `ProcessingRequest.attributes` CEL-attribute-value encoding; `context.Context` for per-stream + per-message cancellable bidi-stream calls (threaded into `*grpc.ClientConn` via `ProcessorClient.Process(ctx)`); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext h2c processor-cluster fixture (NO TLS-to-processor fixture coverage — mirrors phase-18.2 fixture 0021 disposition per parent SPEC §11 + §8 item 17; behavioral verification of envoy-go's bidi-stream over TLS lives in `internal/grpcclient/processor_client_test.go` against a TLS-fronted test gRPC server if the IMPL adds such coverage, otherwise mocked).

---

## Scope check — why phase 19.1 ships as one row (it already is the split half)

Phase 19 was SPLIT into `19.1-http-filter-ext-proc-headers` + `19.2-http-filter-ext-proc-body` at the phase-19 parent SPEC commit (`9cc1458`) per ADR-0045 / ADR-0176 — split by **feature-class** (19.1 = filter scaffold + bidi-stream primitive + JSON codec + headers stages + ADR-0174 encode-side callback symmetry; 19.2 = body-stage activation + ADR-0175 encode-side body-buffering primitive). This PLAN is for the 19.1 sub-phase ONLY; no further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward). The 19.2 sibling stub at `docs/envoy-go/phases/19.2-http-filter-ext-proc-body/README.md` documents the deferred surface; the full 19.2 SPEC is drafted at 19.2's lifecycle-state 1 after 19.1 phase-done.

Net change estimate for 19.1 (mirroring the phase-09..18.2 PLAN component-table convention):

- `internal/filter/http/extproc/doc.go` ~50 (package overview + 6-decision summary)
- `internal/filter/http/extproc/extproc.go` ~750–1150 cumulative across Tasks 2 + 10 + 11 (Task 2 skeleton ~400–600 — filter type + factory stub + `filterStats` (9 counters) + `compiledConfig` shape + compile-time assertions; Task 10 +150–250 — per-route resolution + `newFilterStats` body + cache-on-first-use; Task 11 +200–300 — full `buildCompiledConfig` body + full `DecodeHeaders`/`EncodeHeaders` bodies + functional `New` factory)
- `internal/filter/http/extproc/check.go` ~600–900 (`buildGRPCProcessorClient` + `buildHTTPProcessorClient` + `applyProcessingResponse` + `applyHeaderMutation` + `emitImmediateResponse` + `mode_override` validator + `route_cache_action` translator + failure-mode handler)
- `internal/filter/http/extproc/attributes.go` ~250–400 (`buildAttributeEnvelope` + attribute-name → accessor map + `lowercaseHeaderMap` helper + `sourcePrincipalFirstOrEmpty` helper)
- `internal/filter/http/extproc/processor.go` ~250–400 (state-machine model: `stage` enum + per-direction `ProcessingMode` state + `dispatchStage` + `completeStage` + resume-channel discipline + `OnDestroy` cancel + `CloseSend` + `override_message_timeout` timer-reset)
- `internal/filter/http/extproc/json.go` ~150–250 (`marshalProcessingRequest` + `unmarshalProcessingResponse` + `protojson` MarshalOptions/UnmarshalOptions setup per ADR-0170)
- `internal/filter/http/extproc/extproc_test.go` ~1500–2500 (Groups 1–11 per SPEC §14.1)
- `internal/filter/http/extproc/fuzz_test.go` ~100 (24th fuzzer `FuzzExtProcConfigParse`; corpus seeds covering both service modes + processing_mode permutations + PARSE-REJECT-triggering variants)
- `internal/grpcclient/processor_client.go` ~250–400 NEW (ADR-0169 — `ProcessorClient` struct + `NewProcessorClient(*Dialer, ...)` + `(*ProcessorClient).Process(ctx) (ProcessStream, error)` + `ProcessStream` interface (Send/Recv/CloseSend) + `(*ProcessorClient).Close`; reuses existing `*Dialer`; NO `Dialer` API changes)
- `internal/grpcclient/processor_client_test.go` ~250–400 NEW (Groups 1+2+3 per SPEC §14.1 — `NewProcessorClient` happy + PARSE-REJECT-on-unknown-cluster + PARSE-REJECT-on-UseH2-false; `(*ProcessorClient).Process` bidi-stream lifecycle + Send/Recv/CloseSend round-trip + mid-stream cancel via parent ctx cancellation + per-message timeout propagation; `(*ProcessorClient).Close` idempotency)
- `internal/filter/http/callbacks.go` ~+50–100 (ADR-0174 — 6 new methods on `EncoderFilterCallbacks`: `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`)
- `internal/filter/http/chain.go` ~+50–80 (6 new `*encoderCB` reader methods; NO new chain fields or seeding primitives — the 6 chain fields ALREADY exist per ADR-0165 + are SET-once at HCM dispatch BEFORE either decode or encode dispatch; the encoder-side readers consume the SAME fields verbatim)
- `internal/filter/http/chain_test.go` ~+80–120 (6 new seed-and-read round-trip tests for the encoder-side reader methods; mirrors the `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed` template; PLUS 6 nil/empty fall-through tests)
- `cmd/envoy-go/main.go` ~+1 (`httpReg.Register(extproc.TypeURL, extproc.New)` inserted alphabetical-after `extauthz` and before `fault` per ADR-0100 §2.2)
- `test/helpers/extprocgrpc/doc.go` ~25 NEW
- `test/helpers/extprocgrpc/extprocgrpc.go` ~200–300 NEW (in-process bidi-stream gRPC `ExternalProcessor.Process` server; plaintext h2c on ephemeral port; `:path`-keyed scriptable per-stage `ProcessingResponse` sequences)
- `test/helpers/extprocgrpc/extprocgrpc_test.go` ~120–180 NEW (Server lifecycle + scripted sequence + stop + concurrent client + bidi-stream half-close)
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind` enum value `HTTPExtProcGRPC BackendKind = 19` after `HTTPExtAuthzGRPC = 18`)
- `test/differential/runner_test.go` ~+12 (blank import + switch-case for `HTTPExtProcGRPC`)
- `test/fixtures/0022-http-ext-proc-grpc/` (NEW DIRECTORY) — `envoy.yaml` ~250 + `envoy-go.yaml` ~250 + `expectations.yaml` ~90 + `README.md` ~150 + `inputs/driver.go` ~400 = ~1140 LoC
- `docs/envoy-go/DECISIONS.md` — 8 ADRs landed at 19.1 IMPL (ADR-0167 §Decision + §Consequences at Task 2; ADR-0168 §Decision + §Consequences at Task 2 + Task 11; ADR-0169 §Decision + §Consequences at Task 4; ADR-0170 §Decision + §Consequences at Task 6; ADR-0171 header-mode §Decision + §Consequences at Task 7; ADR-0172 header-mode §Decision + §Consequences at Task 8; ADR-0173 §Decision + §Consequences at Task 10; ADR-0174 §Decision + §Consequences at Task 5); ~+650 LoC. NO new ADR numbers consumed at 19.1 IMPL under the PLAN's hypothesis D12.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+400 (§13 8-edit bundle per SPEC §13)
- `docs/envoy-go/ROADMAP.md` row 19.1 flips `in-progress → done`; row `19` (parent) UNCHANGED at this commit; row `19.2` (planned) UNCHANGED; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place
- `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (NEW) ~700 across 15 task entries
- `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/REVIEW.md` (NEW) ~280

**Production code: ~2400–3700 LoC** (`internal/filter/http/extproc/` ~1700–2700 + `internal/grpcclient/` extension ~250–400 + `internal/filter/http/callbacks.go`+`chain.go` extension ~100–180 + boot-reg ~1) **+ ~350–500 LoC test-helper = ~2750–4200 LoC production** + ~1900–3000 LoC tests + ~1140 LoC fixture + ~1050 LoC docs ≈ **~6840–9390 LoC total**. Task count below is **15** — comfortably under the ADR-0045 25-task split-gate (mirrors phase-18.1's 15-task PLAN exactly). The production-LoC high-end (~4200) is well above the ~1500-LoC soft threshold but the task-count gate is load-bearing, not LoC (per the phase-13..18.x precedent + 19.1 IS ALREADY a sub-phase row), so **19.1 ships as the single row it is** — no further split.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/extproc/doc.go` | NEW | Package doc enumerating: (a) the package is the TWELFTH §9 HTTP filter (after cors/fault/header_mutation/local_ratelimit/csrf/buffer/compressor/bandwidth_limit/rbac/jwt_authn/ext_authz); (b) the BOTH-DECODE-AND-ENCODE shape (FIRST §9 row to participate on both sides — phase-14 compressor was encode-only); (c) the dual-mode `compiledConfig` envelope (`grpc_service` bidi-stream + `http_service` JSON POST per-stage; 19.1 activates BOTH arms; body-mode PARSE-REJECT in 19.1 lifts at 19.2); (d) the per-stage state machine (request_headers → response_headers in 19.1; body stages at 19.2); (e) the 9-counter `filterStats` surface (per parent §5.P4 hypothesis RATIFIED-PENDING-IMPL-TIME); (f) the per-route 5th-canonical REUSE (SECOND CONSECUTIVE §9 row after phase 18; SHARED-stats); (g) the ADR anchors (ADR-0167..ADR-0174 lands at 19.1; ADR-0175 lands at 19.2; ADR-0176 already at parent SPEC commit). Mirrors `internal/filter/http/extauthz/doc.go` shape. ~50 LoC. Per SPEC §1 + §3 + §6. |
| `internal/filter/http/extproc/extproc.go` | NEW | Main file. **Public surface** (per SPEC §6.1): `const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor"` + `func New(cfg *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.HTTPFilter, error)`. **Internal types** per SPEC §6.2: `type filter struct { ... }` (per-stream state — `cc *compiledConfig`, `dcb DecoderFilterCallbacks`, `ecb EncoderFilterCallbacks`, `perRoute *resolvedPerRoute` cached at DecodeHeaders, `activeProcessingMode *resolvedProcessingMode` mutated by mode_override, `streamCtx context.Context` + `streamCancel context.CancelFunc`, `stream ProcessStream` (gRPC mode) OR `httpStream *httpStream` (HTTP mode), `requestContentType string` captured at DecodeHeaders for gRPC-downstream-detection sniff per parent §5.P2, `parentCtx context.Context` from FactoryCtx). `type compiledConfig struct { grpcClient *grpcclient.ProcessorClient; httpClient *httpProcessorClient; httpServiceHeadersOnly bool; processingMode *resolvedProcessingMode; allowModeOverride bool; allowedOverrideModes []*resolvedProcessingMode; failureModeAllow bool; messageTimeout time.Duration; maxMessageTimeout time.Duration; disableImmediateResponse bool; mutationRules *resolvedMutationRules; forwardRules *resolvedForwardRules; requestAttributes []string; responseAttributes []string; routeCacheAction extprocv3.ExternalProcessor_RouteCacheAction; stats *filterStats; statPrefix string }` (per SPEC §6.2 — field-final at 19.1 per ADR-0168 §Decision; 19.2 amends only to lift body-mode PARSE-REJECT). `type filterStats struct { streamsStarted, streamMsgsSent, streamMsgsReceived, spuriousMsgsReceived, streamsFailed, streamsClosed, failureModeAllowed, overrideMessageTimeoutReceived, overrideMessageTimeoutIgnored *stats.Counter }` (9 counters per parent §5.P4 hypothesis; all unconditionally registered at `New()` time per ADR-0173 STRUCTURALLY-UNREACHABLE-counter discipline). **`DecodeHeaders` body** per SPEC §6.3: (1) resolve per-route → cache on `f.perRoute`; (2) if `disabled` → `Continue`; (3) check `request_header_mode` (SKIP → `Continue`); (4) capture `f.requestContentType = headers.Get("content-type")` for gRPC-downstream sniff; (5) build the request_headers ProcessingRequest via `buildRequestHeadersProcessingRequest` (consumes ADR-0165 decoder accessors); (6) `f.openProcessorStream()` (gRPC mode) OR noop (HTTP mode); (7) `f.dispatchStage(stageRequestHeaders, pr)` + return `StopIteration`. **`EncodeHeaders` body** per SPEC §6.4: (1) skip if per-route disabled; (2) check `f.activeProcessingMode.ResponseHeaderMode` (SKIP → `Continue`); (3) build response_headers ProcessingRequest (consumes ADR-0174 encoder accessors); (4) `f.dispatchStage(stageResponseHeaders, pr)` + return `StopIteration`. **`DecodeData` + `DecodeTrailers` + `EncodeData` + `EncodeTrailers`** — pass-through `Continue` (body/trailer modes PARSE-REJECTed at parse time per ADR-0168; in 19.1 these methods are never called for a valid config but are implemented for code-completeness). **`OnDestroy`** invokes `f.streamCancel()` + `f.stream.CloseSend()` (gRPC mode) OR cancels in-flight `http.Client.Do` (HTTP mode). Compile-time: `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` + `var _ envoyhttp.StreamEncoderFilter = (*filter)(nil)` + `var _ envoyhttp.HTTPFilter = (*filter)(nil)`. The `buildCompiledConfig` function is at Task 11 (consumes Tasks 3-10). ~400–600 LoC. Per SPEC §6.1 + §6.2 + §6.3 + §6.4 + §6.10. ADR-0167 §Decision + §Consequences at Task 2; ADR-0168 §Decision + §Consequences at Task 2 (struct + dispatch sketch) + Task 11 (`buildCompiledConfig` body). |
| `internal/filter/http/extproc/check.go` | NEW | The mode-agnostic per-stage dispatcher + the gRPC + HTTP per-mode helpers. **`buildGRPCProcessorClient(gs *core.GrpcService, ctx envoyhttp.FactoryCtx, messageTimeout time.Duration) (*grpcclient.ProcessorClient, error)`** per SPEC §6.5: (1) PARSE-REJECT `GoogleGrpc` arm (`"ext_proc: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"` — mirrors phase-18.2 D7); (2) PGV-mirror `EnvoyGrpc.cluster_name` `min_len: 1` PARSE-REJECT; (3) cluster-manager lookup via `ctx.ClusterManager.Get(cluster_name)` → PARSE-REJECT on unknown cluster OR `!cluster.UseH2()`; (4) construct `*grpcclient.ProcessorClient` via `grpcclient.NewProcessorClient(dialer, cluster_name, messageTimeout)`; (5) SILENT-IGNORE `initial_metadata` + `retry_policy` per parent §4.3. **`buildHTTPProcessorClient(hs *extprocv3.ExtProcHttpService) (*httpProcessorClient, error)`** per SPEC §6.5: (1) validate `HttpService.server_uri.uri` set + non-empty PARSE-REJECT; (2) construct an `*http.Client{Timeout: hs.server_uri.timeout}`; (3) capture `path_prefix` for per-stage URL construction. **`applyProcessingResponse(stage stage, resp *extprocv3.ProcessingResponse) (action, error)`** per SPEC §6.7 — mode-agnostic dispatch: (1) ImmediateResponse → `emitImmediateResponse` (unless `disable_immediate_response` → silent-drop + `spuriousMsgsReceived++`); (2) `override_message_timeout` → `handleOverrideMessageTimeout` (timer reset; other fields ignored per parent §5.P10); (3) `mode_override` → applied only on header-response stages when `allow_mode_override=true` AND `isModeAllowed` per parent §5.P1 RATIFIED-AND-REFINED; silently ignored otherwise (NOT classified as spurious per the proto doc); (4) `CommonResponse` per stage — `request_headers`/`response_headers` cases switch; (5) `applyHeaderMutation(stage, hm)` per parent §5.P3 (per-header `mutation_rules` gating; rejected mutations drop + `spurious_msgs_received++` ONCE per stage with any rejection); (6) `clear_route_cache` + `route_cache_action` precedence — request_headers stage ONLY per parent §5.P5; (7) `CONTINUE_AND_REPLACE` → classify as `spuriousMsgsReceived++` + dispError in 19.1 per planner-time D7 (body-mode dependent; lands at 19.2). **`applyHeaderMutation(stage stage, hm *extprocv3.HeaderMutation)`** iterates `set_headers` + `remove_headers`; per-header `f.cc.mutationRules.IsAllowed(name)` check (per parent §5.P3); allowed → `f.dcb.RequestHeaders().Set/Add/Del` or `f.ecb.ResponseHeaders().Set/Add/Del`; rejected → drop + set per-stage rejection flag for ONCE-per-stage `spurious_msgs_received` increment. **`emitImmediateResponse(ir *extprocv3.ImmediateResponse) action`** per SPEC §4 — extracts status/headers/body/grpc_status/details; applies `*HeaderMutation` SET-or-REMOVE (per parent §5.P2 — distinct from phase-18.2's plain `[]HeaderValueOption`); gRPC-downstream-detection via `f.requestContentType == "application/grpc"` sniff per parent §5.P2: if gRPC downstream, `body` encoded into `grpc-message` header + `grpc_status` → `grpc-status` response trailer + response `content-type: application/grpc`; non-gRPC: `body` emitted with `content-type: text/plain` (default; unless `headers.set_headers[content-type]` overrides); calls `f.dcb.SendLocalReply(status, body, headers)` for decode-stage OR `f.ecb.SendLocalReply(status, body, headers)` for encode-stage (the FIRST §9 row to emit `SendLocalReply` from the encode side at the response_headers stage per parent §7 ADR-0167 — the framework's `SendLocalReply` enters the encode chain at `filter[len-1]` per ADR-0075 supporting this). **`handleOverrideMessageTimeout(ot *durationpb.Duration)`** per parent §5.P10: (a) requires `max_message_timeout >= 1ms` (`overrideMessageTimeoutIgnored++` otherwise); (b) range check `[1ms, max_message_timeout]` (`overrideMessageTimeoutIgnored++` if out of range); (c) at most ONCE per stage; (d) resets the per-stage recv-timer via `context.WithTimeout` cancel-and-rebuild per planner-time D6; (e) `overrideMessageTimeoutReceived++` on entry. **Failure-mode handling**: error paths route through `mapTransportError` — `failure_mode_allow:false` (default) → `SendLocalReply(500, "", {})` (response-headers-not-delivered branch) OR stream-reset (response-headers-delivered branch — IMPL settles via `f.dcb.SendLocalReply(0, ...)` framework primitive per planner-time D5; the existing primitive suffices, NO new ADR fires) + `streams_failed++`; `failure_mode_allow:true` → `Continue` + `failure_mode_allowed++` + `streams_failed++`. ~600–900 LoC. Per SPEC §6.5 + §6.7. ADR-0171 header-mode §Decision + §Consequences at Task 7; ADR-0172 header-mode §Decision + §Consequences at Task 8. |
| `internal/filter/http/extproc/attributes.go` | NEW | The `request_attributes` / `response_attributes` envelope builder. **`buildRequestHeadersProcessingRequest(f *filter, headers http.Header, endStream bool, attributeAllowlist []string) *extprocv3.ProcessingRequest`** per SPEC §6.6 — populates `ProcessingRequest.request_headers = &HttpHeaders{headers: headerMapToHeaderMap(headers), end_of_stream: endStream}` + `ProcessingRequest.attributes` map from the allowlist via the ADR-0165 decoder-side accessors (`f.dcb.DownstreamRemoteAddr()`/`DownstreamLocalAddr()`/`DownstreamTLSServerName()`/`DownstreamTLSPeerCertDER()`/`DownstreamProtocol()`/`ListenerPrincipal()` + `f.dcb.DownstreamPrincipal()`-derived for ADR-0144). **`buildResponseHeadersProcessingRequest(f *filter, headers http.Header, endStream bool, attributeAllowlist []string) *extprocv3.ProcessingRequest`** per SPEC §6.6 — symmetric, populates `ProcessingRequest.response_headers` + `ProcessingRequest.attributes` from the NEW ADR-0174 encoder-side accessors (`f.ecb.DownstreamRemoteAddr()`/`DownstreamLocalAddr()`/...). **Attribute-name hypothesized mapping** per SPEC §6.6 table (RATIFIED-PENDING-IMPL-TIME — parent §5.P4-class closes at Task 13 fixture-harness empirical scrape against reference Envoy v1.37.2's CEL attribute registry): `source.address` (from `DownstreamRemoteAddr()`); `destination.address` (from `DownstreamLocalAddr()`); `connection.requested_server_name` (from `DownstreamTLSServerName()`); `connection.subject_local_certificate` (derived from listener cert + ADR-0144); `request.protocol` (from `DownstreamProtocol()`); `connection.principal` (from `ListenerPrincipal()`); `source.principal` (from `DownstreamPrincipal()[0]` first-or-empty; ENCODE-side has no `DownstreamPrincipal` — empty per planner-time D10 settle "6 methods, not 7"). The codec wraps each attribute value in a `*structpb.Value` per the protobuf CEL attribute encoding. **Helpers added:** `lowercaseHeaderMap(http.Header) map[string]string` (single-value-per-key — multi-value headers join with `,`; mirrors phase-18.2's `lowercaseHeaderMap`); `sourcePrincipalFirstOrEmpty([]string) string`. ~250–400 LoC. CONSUMES Task 5 (encoder-side ADR-0174 methods) — wired at Task 9. |
| `internal/filter/http/extproc/processor.go` | NEW | The bidi-stream + per-stage state machine. **Types:** `type stage int` with `stageRequestHeaders`/`stageResponseHeaders` constants (body/trailer-stages PARSE-REJECT at parse — never reached in 19.1; constants reserved for 19.2 lift); `type action int` with `actContinue`/`actStop`/`actError`/`actImmediate`/`actContinueButStillWaiting` constants; `type resolvedProcessingMode struct { RequestHeaderMode HeaderSendMode; ResponseHeaderMode HeaderSendMode; RequestBodyMode BodySendMode; ResponseBodyMode BodySendMode; RequestTrailerMode HeaderSendMode; ResponseTrailerMode HeaderSendMode }` (the per-direction state — DEFAULT translated per parent §5.P9: header-mode DEFAULT → SEND, trailer-mode DEFAULT → SKIP at parse time). **`(*filter).openProcessorStream()`** per SPEC §6.8: `f.streamCtx, f.streamCancel = context.WithCancel(f.parentCtx); f.stream, _ = f.cc.grpcClient.Process(f.streamCtx)` — first error caught at first Send. **`(*filter).dispatchStage(stage stage, req *extprocv3.ProcessingRequest)`** per SPEC §6.8 — async dispatch: spawns goroutine performing `Send` + `Recv` (gRPC) OR `http.Client.Do` (HTTP); per-message timeout via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` per planner-time D6 + D9; on Send error → `streamsFailed++` + `f.completeStage(stage, nil, err)`; on success `streamMsgsSent++`; on Recv error → `streamsFailed++` + `f.completeStage(stage, nil, err)`; on success `streamMsgsReceived++` + `f.completeStage(stage, resp, nil)`. **`(*filter).completeStage(stage stage, resp *extprocv3.ProcessingResponse, err error)`** — invokes `applyProcessingResponse` synchronously inside the goroutine; on `actContinue` signals `f.dcb.ContinueDecoding()` (decode stages) or `f.ecb.ContinueEncoding()` (encode stages) via the resume channel; on `actImmediate` already emitted; on `actError` applies failure-mode posture. **Mode-override re-eval discipline** per ADR-0171: on header-response paths only; `f.activeProcessingMode` mutation is sequential WRT the dispatch (the framework dispatches decode then encode sequentially per HTTP transaction); race-tests confirm at Task 12. **`override_message_timeout` timer-reset** per planner-time D6: `context.WithTimeout` cancel-and-rebuild — the goroutine's recv-context is cancelled + a fresh `context.WithTimeout(f.streamCtx, newTimeout)` is built (NOT a `time.AfterFunc.Reset` approach — `context.WithTimeout` is the canonical primitive in envoy-go per phase-09/phase-18.x precedent). ~250–400 LoC. Per SPEC §6.8 + §3.3. ADR-0171 header-mode §Decision + §Consequences at Task 7. |
| `internal/filter/http/extproc/json.go` | NEW | The filter-local `ProcessingRequest`/`ProcessingResponse` JSON codec for `http_service` mode per SPEC §3.2 + §6.5. **`marshalProcessingRequest(req *extprocv3.ProcessingRequest) ([]byte, error)`** uses `protojson.Marshaler{UseProtoNames: true, EmitUnpopulated: false, UseEnumNumbers: false}` per ADR-0170 §Decision (the IMPL settles the exact options; PLAN's strong hypothesis is the above per parent §5.P8 + the proto3 JSON canonical mapping). **`unmarshalProcessingResponse(data []byte) (*extprocv3.ProcessingResponse, error)`** uses `protojson.UnmarshalOptions{DiscardUnknown: true}` for forward-compat with future Envoy versions. Per parent §5.P8 RATIFIED-PENDING-IMPL-TIME — the IMPL fixture-harness scrape at Task 13 closes the wire-shape pin against reference Envoy v1.37.2 (one request/response pair captured). On `unmarshalProcessingResponse` failure per planner-time D8: classify as `streamsFailed++` + dispError (fail-loud — NOT silent-drop; `DiscardUnknown: true` covers the forward-compat case). Filter-local for MVP per ADR-0170 §Decision; generalization to `internal/jsoncodec/` deferred to the THIRD consumer trigger per the phase-18.1 ADR-0159 (b)-disposition rationale. ~150–250 LoC. Per SPEC §3.2 + §6.5 + §6.6. ADR-0170 §Decision + §Consequences at Task 6. |
| `internal/filter/http/extproc/extproc_test.go` | NEW | Unit tests per SPEC §14.1 — **Group 1: factory parse paths** (mutual-exclusion both-set/neither-set PARSE-REJECT; body-mode != NONE PARSE-REJECT in 19.1; trailer-mode != SKIP PARSE-REJECT; STREAMED-only flag PARSE-REJECT; GoogleGrpc PARSE-REJECT; route_cache_action + disable_clear_route_cache mutual-exclusion PARSE-REJECT; HTTP-service-mode body-mode PARSE-REJECT per proto constraint; EnvoyGrpc cluster_name empty PARSE-REJECT; unknown cluster PARSE-REJECT; UseH2:false cluster PARSE-REJECT). **Group 2: `compiledConfig` field values post-parse** (gRPC arm; HTTP arm; both arms' processingMode + allowModeOverride + allowedOverrideModes + failureModeAllow + messageTimeout + maxMessageTimeout + disableImmediateResponse + mutationRules + forwardRules + requestAttributes + responseAttributes + routeCacheAction + statPrefix populate correctly; default values match proto defaults — messageTimeout 200ms, maxMessageTimeout 0). **Group 3: `buildGRPCProcessorClient` + `buildHTTPProcessorClient` construction**. **Group 4: `applyHeaderMutation` + `mutationRules` per-header gating** (allowed mutations apply via dcb/ecb headers Set/Add/Del; rejected mutations dropped + `spurious_msgs_received++` ONCE per stage with any rejection; mutation_rules unset → proto-default protected set host/:authority/:scheme/:method/x-envoy-* applies per parent §5.P3). **Group 5: `applyProcessingResponse` per-stage dispatch** (request_headers + response_headers; non-applicable stages classified as `spurious_msgs_received++` + dispError; CONTINUE_AND_REPLACE in 19.1 classified as spurious + dispError per planner-time D7; CONTINUE default action). **Group 6: `emitImmediateResponse` for both decode and encode stages + grpc_status content-type sniff translation** (gRPC downstream: body → grpc-message header + grpc_status → grpc-status response trailer + response content-type application/grpc; non-gRPC: body → text/plain default; `*HeaderMutation.set_headers` SET vs APPEND vs REMOVE per planner-time D4 4-arm dispatch; `details` SILENT-DROPPED per SPEC §2.8 forward-pointer to response_code_details). **Group 7: ProcessingMode resolution + mode_override re-eval** (DEFAULT → SEND for headers / SKIP for trailers per parent §5.P9; mode_override on header-response paths only; mode_override on body/trailer-response paths silently ignored — NOT classified spurious per parent §5.P1; allow_mode_override + allowed_override_modes validation; max_message_timeout >= 1ms gates override_message_timeout enablement; override_message_timeout range check [1ms, max_message_timeout]; out-of-range → `overrideMessageTimeoutIgnored++`). **Group 8: per-route 5th-canonical REUSE + cache-on-first-use + 9-counter SHARED-stats** (per-route disabled → `Continue` immediately + zero counter increments; per-route overrides processing_mode + grpc_service consumed; per-route `[#not-implemented-hide:]` fields silent-ignored — async_mode + request_attributes + response_attributes + metadata_options + grpc_initial_metadata; cache-on-first-use confirmed across ClearRouteCache invocations). **Group 9: error-posture** (`failure_mode_allow:false` + processor unreachable → `SendLocalReply(500, ...)` + `streamsFailed++`; `failure_mode_allow:true` + processor unreachable → `Continue` + `failureModeAllowed++` + `streamsFailed++`; message_timeout exceeded → same as unreachable; `disable_immediate_response:true` + ImmediateResponse arrival → silent-drop + `spuriousMsgsReceived++`). **Group 10: `OnDestroy` cancels in-flight bidi-stream** (cancellation propagates through `*ProcessStream.Recv`; `CloseSend` called once on OnDestroy). **Group 11: attribute envelope builder** (mocked TLS state in *filter; assert ProcessingRequest.attributes content per SPEC §6.6 table; decoder + encoder accessor paths). ~1500–2500 LoC. |
| `internal/filter/http/extproc/fuzz_test.go` | NEW | 24th fuzzer `FuzzExtProcConfigParse` per SPEC §7.3. Fuzzes arbitrary protobytes → `proto.Unmarshal` of `*extprocv3.ExternalProcessor` → wraps in `*anypb.Any` → `New(any, mockFactoryCtx)` → asserts factory returns either `(non-nil HTTPFilter, nil err)` OR `(nil, non-nil err)`; never panics; never blocks. Corpus seeds (8-12 variants): valid grpc_service config + valid http_service config + various processing_mode permutations (incl. PARSE-REJECT-triggering body/trailer/STREAMED variants) + various error-posture configurations (with/without failure_mode_allow, disable_immediate_response, various timeouts) + various per-route configs (disabled:true, overrides{processing_mode}, overrides{grpc_service}) + various mutation_rules + various forward_rules + various route_cache_action values + both-set / neither-set service config (PARSE-REJECT) + invalid GoogleGrpc arm (PARSE-REJECT). 30s/seed under the ADR-0018 budget. The `FuzzProcessingResponseMapping` fuzzer is 19.2's deliverable (the body-stage CommonResponse mappings are part of its scope). ~100 LoC. Per SPEC §7.3. Task 11. |
| `internal/grpcclient/processor_client.go` | NEW | The ADR-0169 typed bidi-stream wrapper EXTENDING the existing `*Dialer`. NEW file alongside the existing `grpcclient.go` (which carries `Dialer` + `AuthClient`) per planner-time decision D13 (NEW file, NOT extending the existing `grpcclient.go` — the file separation mirrors the SPEC §6.9 file-layout sketch "alongside `auth_client.go`" and keeps each wrapper in its own focused file going forward; the existing `grpcclient.go` is unchanged at 19.1 IMPL). **Public surface** per SPEC §3.1: `type ProcessorClient struct { conn *grpc.ClientConn; stub extprocv3.ExternalProcessorClient; target string; perMessageTimeout time.Duration; closeOnce sync.Once }` + `func NewProcessorClient(d *Dialer, clusterName string, perMessageTimeout time.Duration) (*ProcessorClient, error)` (calls `d.DialContext(ctx, clusterName)` → `conn`; wraps with `extprocv3.NewExternalProcessorClient(conn)`) + `func (c *ProcessorClient) Process(ctx context.Context) (ProcessStream, error)` (opens a new bidi-stream `Process` RPC; threads `ctx` into the gRPC stream for cancellation) + `type ProcessStream interface { Send(*extprocv3.ProcessingRequest) error; Recv() (*extprocv3.ProcessingResponse, error); CloseSend() error }` + `func (c *ProcessorClient) Close() error` (idempotent close via `sync.Once`). The per-message timeout applies on the recv path inside the filter (not inside `Process` itself — the filter's `dispatchStage` wraps each Recv with `context.WithTimeout` per ADR-0169 §Decision). One `*grpc.ClientConn` per (cluster_name, compiledConfig) pair created at config-load time; bidi-stream lifetime is per HTTP transaction (one `Process` stream per request); the `*grpc.ClientConn` is reused across streams via gRPC's internal multiplexing. Leaks-on-exit MVP per planner-time D2 (mirrors phase-18.2 D2 + ADR-0158 §Decision (vi)). Cross-phase-reusable for any future bidi-stream gRPC filter (ext_proc is currently the sole bidi-stream consumer; a future streaming-access-log filter would reuse the same wrapper pattern). ~250–400 LoC. Per SPEC §3.1 + §6.5 step 3 + §6.9. ADR-0169 §Decision + §Consequences at Task 4. |
| `internal/grpcclient/processor_client_test.go` | NEW | Unit tests per SPEC §14.1 — **Group 1 (`NewProcessorClient`):** happy path (plaintext h2c cluster via a fake `cluster.Manager` + a real `*grpc.Server` registering `ExternalProcessor.Process` from `extprocv3` — uses a thin local test-server, NOT the production `test/helpers/extprocgrpc/` since that's NEW at Task 13); PARSE-REJECT on unknown cluster name; PARSE-REJECT on `UseH2()==false`. **Group 2 (`Process` bidi-stream lifecycle):** open stream → Send → Recv → CloseSend → stream close (happy round-trip); mid-stream cancel via parent ctx cancellation (the OnDestroy primitive); per-message timeout propagation via `context.WithTimeout` (the filter's discipline); concurrent Send+Recv on a single stream is safe per gRPC docs (the IMPL ensures only one goroutine calls Send and only one calls Recv per stage's dispatch — race-detector clean). **Group 3 (`Close` idempotency):** `Close()` second call returns nil; underlying ClientConn closed only once; concurrent `Close()` calls race-clean under `-race`. ~250–400 LoC. Per SPEC §14.1. |
| `internal/filter/http/callbacks.go` | MODIFIED | Add 6 new methods to `EncoderFilterCallbacks` per ADR-0174 (§5.P12 REFUTED at SPEC time): `DownstreamRemoteAddr() net.Addr`; `DownstreamLocalAddr() net.Addr`; `DownstreamTLSServerName() string`; `DownstreamTLSPeerCertDER() []byte`; `DownstreamProtocol() string`; `ListenerPrincipal() string`. Doc-comments cite ADR-0174 + the cross-phase reuse intent (any future encode-side filter participating in response-stage envelopes — e.g., a future response-mutating filter or response-attribute-emitting filter — reuses the same accessor surface). **NO** `DownstreamPrincipal()` extension to the encoder-side per planner-time D10 settle (decoder-side-specific in the ADR-0144 framing; the 6 ADR-0165 methods suffice for `response_attributes` per the SPEC §6.6 hypothesis; if IMPL Task 9 scrape surfaces a need for `source.principal` on the encode side, ADR-0174's method count goes from 6 to 7 — but the PLAN's strong hypothesis is 6 sufficient). ~+50–100 LoC. Task 5. ADR-0174 §Decision + §Consequences at Task 5. |
| `internal/filter/http/chain.go` | MODIFIED | Add 6 new `*encoderCB` reader methods consuming the EXISTING ADR-0165 chain fields (`downstreamRemoteAddr` / `downstreamLocalAddr` / `downstreamTLSServerName` / `downstreamTLSPeerCertDER` / `downstreamProtocol` / `listenerPrincipal`) — verbatim mirror of the `*decoderCB` reader methods at chain.go (per ADR-0174 §Decision draft: NO new chain fields, NO new seeding primitives, NO new HCM dispatch wiring — the chain fields ALREADY exist per ADR-0165 + are SET-once at HCM dispatch (H1 `connection.go:dispatchRequest` + H2 `h2dispatch.go:WriteH2`) BEFORE either `RunDecodeHeaders` or `RunEncodeHeaders` dispatch; ADR-0071's chain-ownership invariant continues to apply; no race introduced — encoder-side reads happen after the SET completes). ~+50–80 LoC. Task 5. ADR-0174 §Consequences at Task 5. |
| `internal/filter/http/chain_test.go` | MODIFIED | Add 6 seed-and-read round-trip tests for the new encoder-side reader methods + 6 nil/empty fall-through tests. Mirrors the existing `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed` template — `TestEncoderCB_DownstreamRemoteAddr_SeededViaSetDownstreamRemoteAddr_ReturnsSeed` etc. Race-detector clean under `-race`. ~+80–120 LoC. Task 5. |
| `cmd/envoy-go/main.go` | MODIFIED | NEW `httpReg.Register(extproc.TypeURL, extproc.New)` line inserted alphabetical-after `extauthz` and before `fault` per ADR-0100 §2.2 (current 13 filter registrations after router: bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, **extauthz**, **extproc** ← INSERT HERE, fault, header_mutation, jwtauthn, localratelimit, rbac → 14 filter registrations; +1 line; +1 import `extproc "github.com/esalaine/envoy-go/internal/filter/http/extproc"`). Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only. ~+1 LoC + +1 import. Task 2. |
| `test/helpers/extprocgrpc/doc.go` | NEW | Package doc — `// Package extprocgrpc implements a minimal in-process scriptable ExternalProcessor.Process bidi-stream gRPC server for differential fixtures whose driver needs to wire an ext_proc grpc_service endpoint into both envoy.yaml and envoy-go.yaml. Used by phase 19.1 fixture 0022-http-ext-proc-grpc. THE FIRST in-tree bidi-stream gRPC server in envoy-go's test tree (the phase-18.2 test/helpers/extauthzgrpc is unary — extprocgrpc extends to bidi). Lifecycle: spawn-per-fixture; the runner allocates a free TCP port, starts the server via New(t), wires the EnvoyGrpc.cluster_name to a cluster pointing at that port in both yaml configs, runs the scenarios, then stops via Stop(). Plaintext h2c (no TLS); per-:path-discriminator scriptable per-stage ProcessingResponse sequence per planner-time decision D1.` ~25 LoC. |
| `test/helpers/extprocgrpc/extprocgrpc.go` | NEW | In-process scriptable bidi-stream gRPC `ExternalProcessor.Process` server per SPEC §7.4. **Public API:** `type Server struct { addr string; grpcSrv *grpc.Server; scripts map[string][]*extprocv3.ProcessingResponse; mu sync.RWMutex }`. `New(t testing.TB) *Server` — listens on `127.0.0.1:0` (ephemeral); registers `extprocv3.RegisterExternalProcessorServer(grpcSrv, s)`; spawns `grpcSrv.Serve(lis)` in a goroutine; calls `t.Cleanup(s.Stop)`. `(s *Server) Addr() string` returns `lis.Addr().String()`. `(s *Server) Script(discriminator string, responses ...*extprocv3.ProcessingResponse)` — registers an ordered sequence of `ProcessingResponse` values for the discriminator (per planner-time D1: the discriminator is the `:path` extracted from the FIRST `ProcessingRequest` received on the stream — typically request_headers stage with a specific path). `(s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error` — the bidi-stream server method: Recv-loops the per-stage `ProcessingRequest` from client; on first request, extracts `:path` discriminator from `req.GetRequestHeaders().GetHeaders()`; advances the per-discriminator script counter; Sends the next `ProcessingResponse` in the sequence; if `ImmediateResponse` arm in script → Send + return (closes stream); if script exhausted before stream-end → return `status.Errorf(codes.Internal, "no script remaining")` (which the filter maps to `dispError` + `streams_failed`); if `CloseSend` from client → return nil (clean close). `(s *Server) Stop()` — `grpcSrv.GracefulStop()`. Plaintext h2c — `grpc.NewServer()` with no `Creds()` option. ~200–300 LoC. Per SPEC §7.4. Task 13. |
| `test/helpers/extprocgrpc/extprocgrpc_test.go` | NEW | Unit tests: `TestNew_StartsServerOnEphemeralPort` (`Addr()` returns non-empty `host:port`); `TestServer_Script_ReturnsScriptedSequence` (registered script returns per-stage responses in order; unregistered discriminator returns `Internal` no-script error which the filter maps to `dispError`); `TestServer_Process_BidiHalfClose` (client `CloseSend` after first response → server returns nil cleanly + stream closes); `TestServer_Process_ImmediateResponseStopsStream` (`ImmediateResponse` arm in script terminates the stream after Send); `TestServer_Stop_Closes`; `TestServer_ConcurrentClient_NoRace` (under `-race` — two concurrent `Process` streams from independent clients). ~120–180 LoC. |
| `test/differential/fixture/fixture.go` | MODIFIED | NEW `BackendKind` enum value `HTTPExtProcGRPC BackendKind = 19` after `HTTPExtAuthzGRPC BackendKind = 18`. Doc-comment: "HTTPExtProcGRPC reuses the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go` for the upstream route + the NEW extprocgrpc helper at `test/helpers/extprocgrpc/` for the in-process bidi-stream gRPC processor server. 2-cluster topology (three HCM listeners l_test_a/b/c plaintext with `ext_proc → router` filter chain + cluster `c_backend` → echobackend subprocess + cluster `c_ext_proc` → extprocgrpc subprocess with `http2_protocol_options: {}`). NO TLS — phase 19.1 fixture is HTTP/1.1 plaintext downstream + plaintext h2c processor cluster per SPEC §7.2 + parent §8 item 17." ~+15 LoC. Task 13. |
| `test/differential/runner_test.go` | MODIFIED | NEW blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0022-http-ext-proc-grpc/inputs"` (alphabetical-after `0021`). NEW switch-case in the `BackendKind` dispatch for `HTTPExtProcGRPC` reusing the existing `startEchoBackend` helper + spawning an `extprocgrpc.New(t)` instance per-test for the in-process bidi-stream gRPC processor server (scenario 5 stops it before the request to exercise `failure_mode_allow`). ~+12 LoC. Task 13. |
| `test/fixtures/0022-http-ext-proc-grpc/` | NEW DIRECTORY | Differential fixture with 8 scenarios per SPEC §7.1. Plaintext-only topology: 1 echo-backend cluster + 1 processor gRPC cluster (with `http2_protocol_options: {}`) + 3 HCM listeners `l_test_a/b/c`. Per planner-time decision D14: l_test_a hosts scenarios 1/2/3/4/7/8 (`failure_mode_allow:false` gRPC-mode); l_test_b hosts scenario 5 (`failure_mode_allow:true` gRPC-mode + processor-down setup); l_test_c hosts scenario 6 (HTTP-service-mode). Scenarios 7+8 use per-route TPFC `ExtProcPerRoute` (7 = `disabled:true`; 8 = `overrides{processing_mode}`). |
| `test/fixtures/0022-http-ext-proc-grpc/envoy.yaml` | NEW | Reference Envoy bootstrap. 3 HCM listeners `l_test_a/b/c` (plaintext TCP; HCM chain `ext_proc → router`) each with listener-level `ExternalProcessor` config (gRPC-mode for a/b: `grpc_service.envoy_grpc.cluster_name: c_ext_proc`; HTTP-mode for c: `http_service.server_uri.uri: http://127.0.0.1:<port>/process`; per-listener `failure_mode_allow` / `processing_mode` / `request_attributes` / `response_attributes` / `mutation_rules` / `message_timeout` / `max_message_timeout` / `allow_mode_override` / `allowed_override_modes` as needed). Routes: `/scenario1` through `/scenario6` → c_backend with various processor scripted responses; `/disabled` with per-route TPFC `ExtProcPerRoute{disabled: true}` (scenario 7); `/override` with per-route TPFC `ExtProcPerRoute{overrides{processing_mode{request_header_mode: SKIP, response_header_mode: SEND}}}` (scenario 8). Cluster `c_backend` STRICT_DNS → echobackend subprocess. Cluster `c_ext_proc` STRICT_DNS → extprocgrpc subprocess with `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}` (mandatory for gRPC framing). ~250 LoC. Per SPEC §7.2. Task 13. |
| `test/fixtures/0022-http-ext-proc-grpc/envoy-go.yaml` | NEW | Equivalent envoy-go bootstrap. Same 3-listener topology + routes + per-route map; cluster type STATIC. ~250 LoC. Per SPEC §7.2. Task 13. |
| `test/fixtures/0022-http-ext-proc-grpc/inputs/driver.go` | NEW | Go driver issuing the 8 scenarios per SPEC §7.1 mirroring the phase-18.2 driver shape. Functions `runScenario1..runScenario8(ctx, baseURLs, processorBaseURL) error` where `baseURLs` is a map of listener name → URL (l_test_a/b/c). Per-scenario assertion: byte-exact body (allow paths backend-echo verbatim; deny paths the processor's ImmediateResponse.body byte-exact) + response status equivalence + `/stats/prometheus` counter-delta equivalence on the reachable counters + backend-arrival header assertions (scenarios 1+8 upstream header mutations) + downstream-arrival header assertions (scenarios 3+6 response header mutations) + processor-server received-ProcessingRequest content assertions (per-stage discriminator; attributes envelope content per the SPEC §6.6 hypothesis-mapping). **extprocgrpc lifecycle helper** `setupProcessorGRPC(t, ctx, port, scripts)`; teardown via `srv.Stop()`. **Counter-delta helper** `scrapeStats` + `assertCounterDelta` mirrors phase-18.2. The driver pre-populates the 8 scripted `ProcessingResponse` sequences via `srv.Script(":path-discriminator", responses...)` before issuing requests. **Three RATIFIED-PENDING-IMPL-TIME pin closures captured by the driver**: (i) §19.P4 — 9-counter name match scraped against the reference Envoy stats output post-scenario-1; (ii) §19.P7 — per-route cache-on-first-use scenario (scenario 8's `/override` + a mid-stream `clear_route_cache:true` in the processor script + a route-config change that would re-select per-route — assert the per-route stays at its DecodeHeaders-time-resolved value); (iii) §19.P8 — JSON codec wire-shape scrape (scenario 6 HTTP-mode — capture one `ProcessingRequest` POST body + one `ProcessingResponse` body against reference Envoy v1.37.2's HTTP-mode emission; assert byte-equivalent `protojson` rendering). ~400 LoC. Per SPEC §7.1 + §15 item 9. Task 13. |
| `test/fixtures/0022-http-ext-proc-grpc/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7. Documents the 8-scenario equivalence claim + the per-route 5th-canonical scenarios 7+8 + the divergence-window allow-list (`response_code_details` field ABSENT on the envoy-go side per SPEC §2.8; gRPC-mode-specific deferred fields per SPEC §8: `metadata_options`/`filter_metadata`/`ProcessingResponse.dynamic_metadata`/`ProcessingRequest.metadata_context` (empty proto for forward-compat)/`CommonResponse.dynamic_metadata` + `CommonResponse.trailers` (`[#not-implemented-hide:]`)/`HttpHeaders.attributes` (deprecated)/`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` silent-ignored; STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes PARSE-REJECT; trailer-modes != SKIP PARSE-REJECT; STREAMED-only flags PARSE-REJECT; GoogleGrpc PARSE-REJECT; initial_metadata + retry_policy SILENT-IGNORED — documented divergence-window from reference Envoy v1.37.2 which accepts these silently; `CONTINUE_AND_REPLACE` in 19.1 classified as spurious per planner-time D7 — documented divergence-window). ~90 LoC. Per SPEC §7. Task 13. |
| `test/fixtures/0022-http-ext-proc-grpc/README.md` | NEW | Fixture overview + 8-scenario list + reference-config citations + extprocgrpc in-process bidi-stream gRPC server lifecycle notes + three-listener topology rationale (per planner-time D14 — l_test_a hosts main matrix; l_test_b hosts failure_mode_allow with `failure_mode_allow:true`; l_test_c hosts HTTP-service-mode) + per-route 5th-canonical-REUSE discipline note (NO ADR-0125 amendment; ADR-0173 records the explicit no-amendment 5th-canonical-REUSE — SECOND CONSECUTIVE §9 row to REUSE after phase 18) + SHARED-stats discipline + counter-delta assertion discipline + divergence-window note (CONTINUE_AND_REPLACE classified as spurious in 19.1 per D7 — lifted at 19.2; body-mode PARSE-REJECT in 19.1 per ADR-0168 — lifted at 19.2 per AMENDMENT; ADR-0174 encode-side callback symmetry landed at Task 5 — first encode-side `response_attributes` envelope population in envoy-go). ~150 LoC. Per SPEC §7.2. Task 13. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | **8 ADRs** anchored at 19.1 IMPL (per parent SPEC §7 + this PLAN's per-ADR table below): **ADR-0167** §Decision + §Consequences at Task 2 (`internal/filter/http/extproc/` package shape + BOTH-DECODE-AND-ENCODE shape + 9-base-counter filterStats + boot-registration + multi-stage SendLocalReply mechanism + the TWELFTH §9 row framing + FIRST-cross-phase-consumer-of-ADR-0158/0165/0166 framing); **ADR-0168** §Decision + §Consequences at Task 2 (struct + dispatch sketch) + Task 11 (`buildCompiledConfig` body completing the dispatch — the §Decision text covers BOTH commits' worth of code; the §Consequences land at Task 11 once the integration is wired); **ADR-0169** §Decision + §Consequences at Task 4 (`*ProcessorClient` bidi-stream wrapper EXTENDING `*Dialer`); **ADR-0170** §Decision + §Consequences at Task 6 (filter-local JSON codec); **ADR-0171** header-mode §Decision + §Consequences at Task 7 (ProcessingMode state-machine + mode_override discipline); **ADR-0172** header-mode §Decision + §Consequences at Task 8 (CommonResponse mutation + ImmediateResponse-at-headers); **ADR-0173** §Decision + §Consequences at Task 10 (per-route 5th-canonical REUSE + SHARED-stats + 9-counter stat surface); **ADR-0174** §Decision + §Consequences at Task 5 (symmetric `EncoderFilterCallbacks` extension). The implementer at each task EXTENDS the existing §Context-draft (already at the parent SPEC commit per ADR-0044) with the full §Decision + §Consequences bodies. ADR-0176 already landed IN FULL at the parent SPEC commit — UNCHANGED by 19.1 IMPL. ADR-0175 lands at 19.2 IMPL. ~+650 LoC. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 — 8-edit bundle: **§13.1** `## HTTP filter chain` → NEW `### envoy.filters.http.ext_proc` subsection (~120 LoC) covering: the services mutually-exclusive dispatch (grpc_service + http_service); body-mode PARSE-REJECT in 19.1 with forward-pointer to 19.2; trailer-mode PARSE-REJECT permanently; STREAMED-only flag PARSE-REJECT permanently; ProcessingMode state-machine + mode-override discipline + allow_mode_override + allowed_override_modes; CommonResponse header_mutation per direction per stage; multi-stage ImmediateResponse (request_headers + response_headers — body-stage at 19.2); clear_route_cache + route_cache_action precedence + request-headers-stage-only-application; failure_mode_allow / message_timeout / max_message_timeout / disable_immediate_response error posture; gRPC-downstream-detection via content-type sniff for grpc_status translation; per-route 5th-canonical REUSE + SHARED-stats; request_attributes + response_attributes envelope discipline. **§13.2** `## Stat-name mapping` → 77 → ~86 name table extension (~30 LoC) adding 9 new rows under `http.<HCM_stat_prefix>.ext_proc.*` per parent §5.P4 hypothesis (RATIFIED at fixture-harness scrape Task 13). **§13.3** `## Equivalence Matrix` → NEW row for fixture `0022-http-ext-proc-grpc` (~5 LoC) with byte-exact body + status + verbatim header assertions + counter-delta equivalence + ProcessingRequest-content assertion. **§13.4** NEW `### Phase 19.1 forward-pointer notes` subsection (~60 LoC) covering the SPEC §8 18-item deferral list (body-stage activation forward-pointer to 19.2; permanent deferrals for STREAMED/trailer modes + dynamic-metadata family + GoogleGrpc + retry-policy + response_code_details; 1 closure of phase-18.2 forward-pointer = encode-side callback symmetry via ADR-0174). **§13.5** `## HTTPFilterCallbacks` AMENDMENT (~30 LoC) per ADR-0174 — documents the 6 new methods landed on `EncoderFilterCallbacks`; chain-side seeding discipline UNCHANGED from ADR-0165 (chain fields SET-once at HCM dispatch BEFORE either decode or encode dispatch); the AMENDMENT adds only the encode-side reader methods; cross-references ADR-0174 for cross-phase reuse intent. **§13.6** `## Per-route canonical patterns cross-reference` UNCHANGED — phase 19.1 REUSES the existing 5th canonical; ADR-0173 records the explicit no-amendment 5th-canonical-REUSE decision (the absence of a §(xiv) amendment is itself a recorded decision — strengthens the ADR-0125 roster-not-monotonic lesson). NO net edit. **§13.7** `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella EXTENSION (~50 LoC) — documents the NEW `*ProcessorClient` bidi-stream wrapper (ADR-0169) alongside the existing unary `*AuthClient`; same package, same `Dialer` integration, NO `Dialer` API changes; cross-references the public surface at SPEC §3.1; NOTE: this is an extension to the EXISTING phase-18.2 umbrella, NOT a new top-level umbrella — bidi-stream is a usage pattern over the same dial layer, not a new framework family. **§13.8** NEW `### JSON codec note` (~20 LoC) lighter-touch reference under §13.7 umbrella (NOT a new top-level umbrella per the phase-18.1 ADR-0159 (b)-disposition rationale) documenting the filter-local `ProcessingRequest`/`ProcessingResponse` JSON codec (ADR-0170) at `internal/filter/http/extproc/json.go`; forward-pointer: generalization to `internal/jsoncodec/` at the second-consumer trigger. Total ~+400 LoC. Task 14. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `19.1` status `in-progress → done` + summary sharpening (post-impl counts: 15-task PLAN-confirmed + ~2750–4200 LoC production estimate + final 8-ADR roster anchored). Row `19` (parent) UNCHANGED at this commit (`in-progress`); row `19.2` UNCHANGED (`planned`) — per parent SPEC §8 the parent row 19 closes AT THE SAME commit as 19.2's phase-done, NOT at 19.1's. Row `19.1`'s `last-touched` column updated to the 19.1 phase-done date. ~+1 net. Task 14. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state: `active-phase: 19.2-http-filter-ext-proc-body` (the next lifecycle target per parent SPEC §8 — 19.2 is `planned`, depends-on `19.1`, awaiting its own lifecycle-state-1 BRAINSTORM-or-SPEC session); `lifecycle-state: phase 19.1 done; phase 19 parent in-progress; phase 19.2 SPEC pending`; `next-skill: superpowers:brainstorming` (or analog — the 19.2 lifecycle-state 1 session may run a fresh brainstorm-scoped-to-SPEC per the 18.2 precedent per parent SPEC §References to 19.2 stub); `last-commit: <Task 14 squash>`; `next-free ADR: ADR-0177` (the SPEC-time closure of §19.P11/§19.P12 RIDS the most-likely escape-valve surfaces; the PLAN's hypothesis D12 is that NO additional ADR fires at 19.1 IMPL — if confirmed, ADR-0177 is the next-free post-19.1 ADR for ADR-0175 at 19.2 + any 19.2-IMPL-unanticipated ADRs); `last-updated: <impl-date>`. Task 14. |
| `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror the phase-09..18.2 PROGRESS.md structure. ~700 LoC across 15 task entries. Task 1 creates the preamble + 17-precondition verification capture; subsequent tasks each append their entry. |
| `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~280 LoC. Task 15. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's eight deferred decisions before implementation; this PLAN settles all eight plus a handful that emerged at PLAN-drafting time. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each task can act without re-deriving them:

1. **D1 — `test/helpers/extprocgrpc/` discriminator + helper API LOCKED per SPEC §7.4 + §12 item 1.** Script discriminator: the `:path` value extracted from `req.GetRequestHeaders().GetHeaders()` on the FIRST `ProcessingRequest` received on the stream (typically the request_headers stage with a specific path; the `:path` is stable for the lifetime of the bidi-stream since one stream serves one HTTP transaction). API surface per SPEC §7.4 sketch: `New(t testing.TB) *Server` returning a started server bound to `127.0.0.1:0`; `(*Server).Addr() string`; `(*Server).Script(discriminator string, responses ...*extprocv3.ProcessingResponse)` (variadic — register an ORDERED SEQUENCE of responses per discriminator, advancing the per-discriminator counter on each Recv); `(*Server).Stop()`. Lifecycle: spawn-per-fixture via `t.Cleanup(s.Stop)`. Plaintext h2c (no TLS); fixture 0022 uses a plaintext processor cluster per SPEC §7.2 + parent §8 item 17. *Anchored: SPEC §7.4 + §12 item 1.*

2. **D2 — `*grpc.ClientConn` close-on-process-exit discipline LOCKED at MVP leaks-on-exit per SPEC §12 item 2.** No `os.Exit` cleanup hook; no `cleanup` package registration. The `*grpc.ClientConn` is owned by the `*compiledConfig` (captured by the `*ProcessorClient` closure); on process exit, the OS reclaims the connection. Rationale: mirrors phase-18.2 D2 + ADR-0158 §Decision (vi) leaks-on-exit discipline; envoy-go has no config hot-reload yet (xDS-CDS deferred per SPEC §8 item 16); the per-(cluster, compiledConfig) ClientConn lifetime is process-bounded. A future hot-reload phase will land a close-on-replacement discipline per a new ADR (NOT 19.1). *Anchored: SPEC §3.1 + §12 item 2 + phase-18.2 D2 precedent.*

3. **D3 — `request_attributes`/`response_attributes` exact accessor map LOCKED per SPEC §6.6 hypothesis-mapping per SPEC §12 item 3.** Settle: the SPEC §6.6 hypothesis-table is the IMPL starting point — `source.address`/`destination.address`/`connection.requested_server_name`/`connection.subject_local_certificate`/`request.protocol`/`connection.principal` populate from the ADR-0165 decoder-side accessors (decode stage) + ADR-0174 encoder-side accessors (encode stage); `source.principal` populates from `DownstreamPrincipal()[0]` on decode-side; ENCODE-side has no `DownstreamPrincipal` per planner-time D10 — `source.principal` is empty on encode-side (decided at IMPL whether to extend D10 to 7 methods if reference Envoy populates `source.principal` at response_headers stage). The IMPL fixture-harness empirical scrape at Task 13 closes the pin RATIFIED per parent §5.P4-class — captures one `ProcessingRequest` with the full attribute envelope against reference Envoy v1.37.2 and asserts byte-equivalent. If the scrape surfaces a CEL-attribute-name divergence (e.g., reference Envoy emits `connection.tls_version` derived from a different accessor than `DownstreamTLSServerName()`), the IMPL adjusts the attribute-name → accessor mapping in `attributes.go` and re-runs the scrape. *Anchored: SPEC §6.6 + §12 item 3.*

4. **D4 — `*HeaderMutation.set_headers` `HeaderValueOption.append_action` 4-arm dispatch table LOCKED per SPEC §12 item 4 + phase-18.2 D5 precedent.** The four enum values: `APPEND_IF_EXISTS_OR_ADD` (default; index 0) → `headers.Add(name, value)` (append-discipline); `OVERWRITE_IF_EXISTS_OR_ADD` (index 1) → `headers.Set(name, value)` (overwrite-discipline); `OVERWRITE_IF_EXISTS` (index 2) → `if len(headers.Values(name)) > 0 { headers.Set(name, value) }` (SET-IF-PRESENT semantic — only overwrites if the header is already present, does NOT add); `ADD_IF_ABSENT` (index 3) → `if len(headers.Values(name)) == 0 { headers.Set(name, value) }` (ADD-IF-ABSENT semantic — adds only when the header is absent, does NOT overwrite). The phase-10 header_mutation enum-handling precedent + phase-18.2 D5 settle is the model. The unit-test Group 4 covers all 4 arms. **Implementation note:** the IMPL may inline the 4-arm switch directly inside `applyHeaderMutation` (cleaner than a discriminator struct field; phase-18.2 extended the `headerKV` struct with a discriminator — but that pattern was driven by the `applyUpstreamMutations` reuse on the deny side, which doesn't apply to ext_proc's stage-local mutation). The IMPL settles the exact representation; behavior is the same. *Anchored: SPEC §12 item 4 + phase-18.2 D5 + phase-10 header_mutation precedent.*

5. **D5 — Reset-vs-local-reply on error after response-headers-delivered LOCKED at "existing framework primitive suffices; NO new ADR fires" per SPEC §12 item 5.** The proto `failure_mode_allow` doc states "if they have been delivered, then instead the HTTP stream to the downstream client will be reset". Settle: the existing framework primitive `f.dcb.SendLocalReply(0, ...)` (with status 0 as the framework's stream-reset signal per the phase-04 + phase-09 + phase-11 + phase-12 + phase-16 + phase-17 + phase-18 deny-path precedents) suffices for the response-headers-delivered branch — the IMPL routes through `SendLocalReply(0, "", {})` to invoke the framework's stream-reset. NO new framework primitive needed; NO new ADR fires. The 19.1 fixture exercises ONLY the request_headers-stage error branch (the response-stage-error branch is not in the 19.1 scenario matrix per SPEC §4 — reserved for IMPL-time verification in unit tests OR a 19.2 fixture-extension scenario). *Anchored: SPEC §12 item 5 + SPEC §4 fixture-scenario commentary.*

6. **D6 — `override_message_timeout` timer-reset implementation LOCKED at `context.WithTimeout` cancel-and-rebuild per SPEC §12 item 6.** Settle: the per-stage recv timeout is implemented via `context.WithTimeout(f.streamCtx, f.cc.messageTimeout)` (the default); when `ProcessingResponse.override_message_timeout` arrives, the goroutine's current recv-context is cancelled (`recvCancel()`) + a fresh `context.WithTimeout(f.streamCtx, newTimeout)` is built for the SUBSEQUENT recv calls in the same stage. NOT `time.AfterFunc.Reset` (which would require a separate timer-state struct and complicates cancellation propagation). `context.WithTimeout` is the canonical primitive in envoy-go per phase-09 + phase-18.x precedent. The override timeout is at most ONCE per stage (per parent §5.P10) — the IMPL tracks a per-stage `overrideApplied bool` flag to enforce this. *Anchored: SPEC §12 item 6 + parent §5.P10.*

7. **D7 — `CONTINUE_AND_REPLACE` in 19.1 disposition LOCKED at classify-as-error-with-spurious-counter per SPEC §12 item 7 + §6.7 sketch.** Settle per SPEC §6.7 sketch: the `applyProcessingResponse` switch on `CommonResponse.status` classifies `CONTINUE_AND_REPLACE` as `f.cc.stats.spuriousMsgsReceived.Inc()` + returns `(actError, errors.New("ext_proc: CONTINUE_AND_REPLACE not supported in 19.1 (lands at 19.2 with body-mode activation)"))`. The processor MUST NOT send CONTINUE_AND_REPLACE in 19.1 since body modes PARSE-REJECT at parse-time; receipt at runtime is a protocol violation by the processor. Documented in BEHAVIOR_CONTRACT §13.1 + §13.4 as a divergence-window from reference Envoy v1.37.2 which accepts CONTINUE_AND_REPLACE at the header stages (in Envoy's full body-mode-active impl, CONTINUE_AND_REPLACE at a header stage triggers the body-replacement discipline). In 19.1 + 19.2 progression: 19.2 IMPL flips this disposition — CONTINUE_AND_REPLACE becomes consumed (`ADR-0172 body-mode AMENDMENT` lifts the PARSE-REJECT). *Anchored: SPEC §12 item 7 + §6.7 + 19.2 forward-pointer.*

8. **D8 — JSON codec error handling on `unmarshalProcessingResponse` failure LOCKED at fail-loud per SPEC §12 item 8.** Settle: on `unmarshalProcessingResponse` returning a non-nil error (malformed JSON from the processor; transport-truncated body; non-protobuf-conformant bytes), classify as `f.cc.stats.streamsFailed.Inc()` + return `(actError, err)` from the http_service-mode dispatch path. The `protojson.UnmarshalOptions{DiscardUnknown: true}` covers the forward-compat case (future Envoy proto extensions land as silently-ignored unknown fields); anything that fails `DiscardUnknown: true` Unmarshal is a hard malformation worth surfacing. The failure routes through the same failure-mode-allow posture as a transport-level error per ADR-0171 — `failure_mode_allow:false` → `SendLocalReply(500)` + `streams_failed++`; `failure_mode_allow:true` → `Continue` + `failure_mode_allowed++` + `streams_failed++`. *Anchored: SPEC §12 item 8.*

9. **D9 — Bidi-stream cancellation race discipline + concurrent decode/encode dispatch LOCKED at "rely on framework sequential decode→encode dispatch + per-stream context.WithCancel for cross-stage cancellation; NO per-stream mutex on Send/Recv" per planner-time emerge (NEW; surfaces at PLAN-time).** Settle: (a) the gRPC library documents that concurrent `Send` and `Recv` on a single `ClientStream` ARE safe (one goroutine for Send, one for Recv); the IMPL keeps ONE goroutine alive per stage's outbound dispatch performing both Send + Recv sequentially → no concurrent Send-vs-Send OR Recv-vs-Recv on the same stream. (b) The framework dispatches `RunDecodeHeaders` BEFORE `RunEncodeHeaders` sequentially per HTTP transaction (verified by the existing 07.1 framework + the chain.go primitives) — the decode-stage dispatch goroutine completes (signals `ContinueDecoding`) BEFORE the encode-stage dispatch goroutine begins. The shared `*ProcessStream` is accessed by AT MOST ONE goroutine at any time → no per-stream mutex needed. (c) `OnDestroy`-driven cancellation: `f.streamCancel()` (the per-stream context's cancel) propagates to any in-flight `Send`/`Recv` via gRPC's context-cancel mechanics; `f.stream.CloseSend()` signals end-of-stream from the client side. The IMPL guards `streamCancel + CloseSend` with `sync.Once` to make `OnDestroy` idempotent. (d) `f.activeProcessingMode` mutation (on mode_override-arrival, on the request_headers-stage recv goroutine) is READ on the encode-stage dispatch path (a different goroutine, BUT scheduled AFTER the decode-stage goroutine completes per (b)) — no atomic load/store needed; the framework's sequential decode→encode dispatch provides happens-before ordering. Race-test at Task 12 covers (a)+(b)+(c)+(d) under `-race`. *Anchored: SPEC §14.2 race-detector concerns + parent §5.P10 bidi-stream lifecycle + planner-time clarification.* **NO ADR fires** — the discipline is a settling of existing primitives (gRPC `ClientStream` semantics + `context.WithCancel` + the framework's sequential dispatch).

10. **D10 — ADR-0174 encoder-side method count LOCKED at 6 (NOT 7) per SPEC §3.3 hypothesis + planner-time settle.** Settle: ADR-0174's encoder-side extension adds exactly 6 methods — `DownstreamRemoteAddr`/`DownstreamLocalAddr`/`DownstreamTLSServerName`/`DownstreamTLSPeerCertDER`/`DownstreamProtocol`/`ListenerPrincipal` (mirroring the 6 ADR-0165 decoder-side methods, NOT the 7 = 6 + `DownstreamPrincipal`). Rationale: `DownstreamPrincipal()` is decode-side-specific per ADR-0144's framing ("the decode-side discovers the principal candidates at dispatch" pattern is one-direction; the principal candidates are seeded at dispatch from the decode-side connection state and are stable for the lifetime of the stream — so the encode-side could in principle re-read them via the same chain field, but the discipline SO FAR has been one-direction). The `response_attributes` envelope's `source.principal` (per SPEC §6.6 table) is populated from `DownstreamPrincipal()` on decode-side; on encode-side it stays empty under the 6-method hypothesis. If at IMPL Task 9 (`attributes.go`) the implementer finds that reference Envoy populates `source.principal` at the response_headers stage (via the same `DownstreamPrincipal`-derived value as request_headers), ADR-0174's method count goes from 6 → 7 + the IMPL adds the 7th method + the Group 12 chain_test gains a 7th seed-and-read test. PLAN's strong hypothesis: 6 suffices (the IMPL settles definitively at Task 9 against the fixture-harness scrape; the cost of adding a 7th method later is small). *Anchored: SPEC §3.3 + planner-time emerge.*

11. **D11 — `internal/grpcclient/` extension via NEW file vs extending the existing `grpcclient.go` LOCKED at NEW file `processor_client.go` per SPEC §6.9 + planner-time emerge.** Settle: ADR-0169's `*ProcessorClient` wrapper lands in a NEW file `internal/grpcclient/processor_client.go` ALONGSIDE the existing `grpcclient.go` (which currently carries BOTH `Dialer` AND `AuthClient` per the as-built phase-18.2 IMPL — the SPEC §6.9 file-layout sketch references "alongside `auth_client.go`" which DOES NOT EXIST as a separate file; the actual phase-18.2 IMPL co-located `AuthClient` in `grpcclient.go`). Settle: create `processor_client.go` as the dedicated file for `ProcessorClient`; LEAVE `grpcclient.go` untouched (it continues to host `Dialer` + `AuthClient`). Rationale: keeping each typed-wrapper in its own file makes future wrapper additions (e.g., a future streaming-access-log filter's `*AccessLogClient`) trivially additive; the existing `grpcclient.go`'s 2-type co-location is grandfathered, not the pattern for new wrappers. The PLAN's File-structure table reflects this (no edits to `internal/grpcclient/grpcclient.go`). The §13.7 BEHAVIOR_CONTRACT extension EXTENDS the existing phase-18.2 umbrella, mentioning the per-wrapper-per-file convention going forward. *Anchored: SPEC §6.9 + planner-time emerge (resolves the SPEC's "alongside auth_client.go" reference vs the as-built layout).*

12. **D12 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that NO additional ADR fires at 19.1 IMPL (NEW; surfaces at PLAN-time).** Per the SPEC-time scrape closure of §19.P11 + §19.P12 (BOTH conditional ADRs REFUTED → fire load-bearing AT SPEC TIME, not at IMPL time per BRAINSTORM §11 lesson (h) — the most-likely escape-valve surfaces are REMOVED). PLAN's strong hypothesis: NO additional ADR fires at 19.1 IMPL — next-free ADR-0177 stays unconsumed at 19.1 phase-done; ADR-0177 is reserved for 19.2 (which lands ADR-0175 as anchored + may consume ADR-0177 for an IMPL-unanticipated surface at its IMPL). The remaining possible 19.1 IMPL surfaces are: (i) bidi-stream cancellation discipline + HCM stream-lifecycle interaction — settled at D9 above with NO ADR (existing primitives suffice); (ii) JSON codec wire-shape divergence — closes at Task 13 fixture-harness scrape; if Envoy's wire-shape diverges from `protojson` defaults, ADR-0170 §Decision is AMENDED in-place at Task 6 per ADR-0044 (NO new ADR — in-place AMENDMENT of an existing landed ADR); (iii) route_cache_action interaction — settled by parent §5.P5 RATIFIED; if Task 13's mid-stream `ClearRouteCache` scenario surfaces a re-resolution-cadence delta from reference Envoy, ADR-0173 §Decision is AMENDED in-place; (iv) `request_attributes`/`response_attributes` CEL-attribute-name allowlist semantics — settled by D3 above; if reference Envoy's CEL registry differs from the SPEC §6.6 hypothesis, the IMPL adjusts `attributes.go` (NOT a new ADR — IMPL-level mapping change). If at IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure), it is ADR-0177 + the PLAN's D12 hypothesis is recorded as falsified in PROGRESS.md. *Anchored: parent SPEC §7 ADR-0044 escape-valve note + SPEC §10 + BRAINSTORM §11 lesson (h).*

13. **D13 — Three-listener fixture topology LOCKED per SPEC §7.2 + planner-time emerge (NEW; surfaces at PLAN-time).** Settle: Fixture 0022 wires 3 HCM listeners `l_test_a/b/c` to separate scenarios per their listener-level config requirements. l_test_a hosts scenarios 1/2/3/4/7/8 (`failure_mode_allow:false` + gRPC-mode `grpc_service.envoy_grpc.cluster_name: c_ext_proc`); l_test_b hosts scenario 5 (`failure_mode_allow:true` + gRPC-mode + processor-down setup — the driver stops the in-process bidi-stream gRPC server BEFORE the request issues, mirroring the phase-18.2 fixture-0021 auth-down treatment); l_test_c hosts scenario 6 (HTTP-service-mode `http_service.server_uri.uri` + headers-only per the proto constraint). The three-listener topology mirrors phase-18.2 fixture 0021's pattern (the 18.1 SPEC §10 notable lesson — `failure_mode_allow` is per-listener, cannot be per-route-overridden). Scenarios 7+8 use per-route TPFC `ExtProcPerRoute` on l_test_a (7 = `disabled:true`; 8 = `overrides{processing_mode{request_header_mode: SKIP, response_header_mode: SEND}}`). *Anchored: SPEC §7.2 + phase-18.2 D10 precedent.*

14. **D14 — Fixture 0022 IS plaintext-only — NO PKI, NO TLS-to-processor fixture coverage (NEW; surfaces at PLAN-time).** Settle: Like fixture 0021 (phase-18.2), fixture 0022 wires plaintext HTTP/1.1 listeners + plaintext h2c processor cluster (per parent §8 item 17 + SPEC §7.2). Behavioral verification of envoy-go's bidi-stream over TLS lives in `internal/grpcclient/processor_client_test.go` unit tests (if the IMPL adds TLS-fronted coverage at Group 1; OPTIONAL — the PLAN does NOT require TLS unit-test coverage at 19.1, since the underlying `*Dialer` already routes through cluster-manager TLS per phase-18.2 ADR-0158 §11.P13 RATIFICATION which is unchanged at 19.1). AttributeContext-side TLS-aware fields (`tls_session.sni`, `source.certificate`) are unit-tested against MOCKED `*filter` state per SPEC §14.1 Group 11. A future integration test MAY close the differential gap if a behavior delta surfaces; the current scope DEFERS this per the cost-vs-coverage tradeoff. *Anchored: SPEC §7.2 + parent §8 item 17 + phase-18.2 D13 precedent.*

---

## ADRs introduced by this plan

The 19.1-landing ADRs anticipated by parent SPEC §7 (ADR-0167 + ADR-0168 + ADR-0169 + ADR-0170 + ADR-0171 header-mode + ADR-0172 header-mode + ADR-0173 + ADR-0174) — **§Context drafts already at the parent SPEC commit `9cc1458`** per ADR-0044 ADR-on-impl convention; **§Decision + §Consequences land at each ADR's Lands-in-Task at 19.1 IMPL**. ADR-0176 (the ADR-0045 split-application ADR) landed IN FULL at the parent SPEC commit — UNCHANGED by 19.1 IMPL. ADR-0175 (the encode-side body-buffering primitive) lands at 19.2 IMPL — UNCHANGED by 19.1 IMPL (its §Context is anchored at the parent SPEC commit but the body authoring is 19.2's work). PLAN's strong hypothesis per D12: **NO conditional impl-time-unanticipated ADR fires at 19.1 IMPL** (next-free ADR-0177 stays unconsumed at 19.1 phase-done).

| ADR | Subject (19.1 portion) | Lands-in-task |
|---|---|---|
| **ADR-0167** | `internal/filter/http/extproc/` package shape — single-token directory (underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/` + `extauthz/`) + BOTH-DECODE-AND-ENCODE `HTTPFilter` (FIRST §9 row to ship both — phase-14 compressor's encode-only + all-others' decode-only are the prior precedents) + 9-base-counter `filterStats` registered unconditionally at `New()` time + boot-registration alphabetical between `extauthz` and `fault` + multi-stage `SendLocalReply` mechanism (request_headers + response_headers in 19.1; body stages at 19.2 — FIRST §9 row whose deny-path can fire at multiple stages) + the TWELFTH §9 row framing + FIRST-cross-phase-consumer-of-ADR-0158/0165/0166 framing + the bidi-stream-framework-lift framing | Task 2 |
| **ADR-0168** | `compiledConfig` shape + the `grpc_service`-vs-`http_service` mutually-exclusive top-level field dispatch (NOT a proto oneof per parent §5.P1) + `processorClient` interface (both arms produce it from config-load time) + the http_service proto-constraint (PARSE-REJECT body/trailer modes when http_service is set) + body-mode 19.1 PARSE-REJECT (§Decision AMENDED at 19.2 to lift) + trailer-mode PARSE-REJECT permanently + STREAMED-only flag PARSE-REJECT permanently (`observability_mode` / `send_body_without_waiting_for_header_response` / `deferred_close_timeout`) + consumed-vs-deferred field discipline + the error-posture fields (`failure_mode_allow` / `message_timeout` / `max_message_timeout` / `disable_immediate_response`) + GoogleGrpc arm PARSE-REJECT inherited from ADR-0157 §Decision AMENDMENT + `initial_metadata` + `retry_policy` SILENT-IGNORED | Task 2 (struct + dispatch sketch); Task 11 (`buildCompiledConfig` body) |
| **ADR-0169** | `*ProcessorClient` bidi-stream wrapper EXTENDING `internal/grpcclient/Dialer` (ADR-0158 §Consequences anchored this cross-phase shape — NO `Dialer` API changes). NEW typed wrapper alongside the existing unary `*AuthClient` — same package, same `Dialer` integration. Public surface: `NewProcessorClient(*Dialer, clusterName, perMessageTimeout)` + `(*ProcessorClient).Process(ctx) (ProcessStream, error)` + `ProcessStream.{Send/Recv/CloseSend}` bidi-stream lifecycle (per HTTP transaction) + `(*ProcessorClient).Close` idempotent. Per planner-time D11: NEW file `processor_client.go` (alongside existing `grpcclient.go`). Cross-phase-reusable for any future bidi-stream gRPC filter | Task 4 |
| **ADR-0170** | `ProcessingRequest`/`ProcessingResponse` JSON codec for http_service mode. Uses `protojson` (already in dependency tree). Filter-local in MVP (per the phase-18.1 ADR-0159 (b)-disposition rationale; generalization to `internal/jsoncodec/` deferred to the THIRD consumer trigger). `protojson` MarshalOptions: `UseProtoNames: true` + `EmitUnpopulated: false` + `UseEnumNumbers: false` per parent §5.P8 hypothesis. UnmarshalOptions: `DiscardUnknown: true` for forward-compat. Wire-shape RATIFIED-PENDING-IMPL-TIME — closes at Task 13 fixture-harness scrape (one request/response pair against reference Envoy v1.37.2). On unmarshal failure per D8: classify as `streamsFailed++` + dispError (fail-loud) | Task 6 |
| **ADR-0171** (header-mode portion) | `ProcessingMode` state-machine + mode-override discipline. Per-direction ProcessingMode state; bidi-stream single-in-flight-message correlation; mid-stream `mode_override` re-eval (header-response paths only per parent §5.P1 — body/trailer-response paths silently ignored, NOT classified spurious); `allow_mode_override` + `allowed_override_modes` validation; `max_message_timeout >= 1ms` gates `override_message_timeout` API enablement; `override_message_timeout` range check `[1ms, max_message_timeout]` (out-of-range → `overrideMessageTimeoutIgnored++`); at most ONCE per stage; per-stage timer-reset via `context.WithTimeout` cancel-and-rebuild per D6; STREAMED-only flags PARSE-REJECT; DEFAULT translates to SEND for headers / SKIP for trailers per parent §5.P9. §Decision AMENDED at 19.2 for body-mode | Task 7 |
| **ADR-0172** (header-mode portion) | `CommonResponse` mutation + `ImmediateResponse` multi-stage deny discipline (header-mode portion). `header_mutation` set/remove per direction per stage; `mutation_rules` per-header gating per parent §5.P3 (allowed mutations apply; rejected mutations dropped + `spurious_msgs_received++` ONCE per stage with any rejection; built-in protected set host/:authority/:scheme/:method/x-envoy-* applies when mutation_rules unset; operator's mutation_rules SUPERSEDES the proto-default set); `clear_route_cache` + `route_cache_action` precedence per parent §5.P5 (BOTH set → PARSE-REJECT; either alone honored; neither → DEFAULT; request_headers stage ONLY); `ImmediateResponse{status, headers, body, grpc_status, details}` with `*HeaderMutation` SET/REMOVE per parent §5.P2 — distinct from phase-18.2's plain `[]HeaderValueOption`; gRPC-downstream-detection via request `content-type: application/grpc` sniff for grpc_status translation per parent §5.P2; CONTINUE_AND_REPLACE classified as `spuriousMsgsReceived++` + dispError in 19.1 per D7 (§Decision AMENDED at 19.2 for body-mode + CONTINUE_AND_REPLACE consumed). §Decision AMENDED at 19.2 for body_mutation + body-stage immediate_response | Task 8 |
| **ADR-0173** | Per-route 5th-canonical REUSE classification (explicit no-new-canonical decision; **NO ADR-0125 amendment paragraph** — SECOND CONSECUTIVE §9 family-row after phase 18 to REUSE; the absence of a §(xiv) amendment is itself a recorded decision — strengthens the ADR-0125 roster-not-monotonic lesson) + SHARED-stats discipline (per-route adjusts `processing_mode`/`grpc_service` but spawns no new stateful policy-evaluation surface) + the `ExtProcOverrides` narrower-override surface (MVP-CONSUMED `processing_mode` + `grpc_service`; `async_mode` + `request_attributes` + `response_attributes` + `metadata_options` + `grpc_initial_metadata` silent-ignored — the per-route `request_attributes`/`response_attributes` at #3/#4 are flagged `[#not-implemented-hide:]` per parent §5.P6, distinct from the top-level `ExternalProcessor.request_attributes`/`response_attributes` at #5/#6 which ARE MVP-consumed) + the 9-counter stat surface (per parent §5.P4 hypothesis; HCM-rooted SN2-reuse `http.<HCM_stat_prefix>.ext_proc.*`; RATIFIED-PENDING-IMPL-TIME — closes at Task 13 fixture-harness scrape per phase-16 §10 lesson (c)) + cache-on-first-use per parent §5.P7 (closes at Task 13 mid-stream-ClearRouteCache scenario) + PGV wrinkles (`disabled` PGV `const: true`; `override` oneof PGV-required) | Task 10 |
| **ADR-0174** | Symmetric `EncoderFilterCallbacks` extension — 6 new methods (per planner-time D10; NOT 7) mirroring ADR-0165's decode-side: `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. NO new chain plumbing — the chain fields ALREADY exist per ADR-0165 + are SET-once at HCM dispatch BEFORE either decode or encode dispatch; the new `*encoderCB` reader methods consume the SAME chain fields verbatim. Required for `response_attributes` envelope population at the response_headers stage. ADR-0044 escape-valve firing at SPEC time per BRAINSTORM §11 lesson (h) — REFUTED at parent §5.P12 SPEC-time scrape → fires load-bearing. Cross-phase-reusable for any future encode-side filter needing socket/TLS/listener state | Task 5 |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the parent SPEC commit per ADR-0044), includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-0XX' docs/envoy-go/DECISIONS.md` returning the expected match count.

**NO in-place ADR-0125 amendment required by phase 19.1** (5th-canonical-REUSE recorded at ADR-0173 — the absence of a §(xiv) amendment is itself a decision; the SECOND CONSECUTIVE §9 row after phase 18 to REUSE the 5th canonical strengthens the ADR-0125 roster-not-monotonic lesson).

**ADR-0044 escape-valve held in reserve per D12** — `ADR-0177` is reserved for 19.2 + any 19.2-IMPL-unanticipated surface. If at 19.1 IMPL time a surface DOES warrant a new ADR (highly unlikely per the SPEC-time scrape closure of §19.P11/§19.P12), it is ADR-0177 + the PLAN's D12 hypothesis is recorded as falsified in PROGRESS.md. If ADR-0173 / ADR-0170 require IMPL-time §Decision AMENDMENTS (e.g., wire-shape divergence at §19.P8 scrape; route_cache_action delta at §19.P7 scrape), the AMENDMENT lands in-place — NO new ADR number consumed.

---

## Task graph (sequential vs parallelizable)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph:

- **Task 1** (PROGRESS.md + 17-precondition verification) — sequential prerequisite for everything; sets up the append-only log.
- **Task 2** (`extproc/{doc.go, extproc.go}` skeleton + filterStats + boot-registration + TypeURL) — sequential; establishes the package + the factory shape + the compileable skeleton (factory returns `nil, errors.New("not implemented")` for now; compile-time interface assertions pass).
- **Tasks 3, 4, 5** — **PARALLELIZABLE** (independent surfaces; all depend on Task 2 for the package being established but NOT on each other):
  - **Task 3** — go.mod / proto-package import wiring (ensures `envoy/extensions/filters/http/ext_proc/v3` + `envoy/service/ext_proc/v3` are reachable + `google.golang.org/grpc` v1.70.0 stays direct dep + `google.golang.org/protobuf/encoding/protojson` reachable; no code).
  - **Task 4** — `internal/grpcclient/processor_client.go` + tests (ADR-0169 — completely independent of the extproc package; lands the bidi-stream wrapper).
  - **Task 5** — `internal/filter/http/callbacks.go` + `chain.go` + `chain_test.go` extension (ADR-0174 — completely independent of the extproc package; lands the 6 new encoder-side reader methods). **PRE-REQUISITE for Task 9** (`attributes.go`'s encoder-side accessor consumption).
- **Task 6** (`extproc/json.go` ADR-0170) — depends on Task 2 only; parallelizable with Tasks 3+4+5.
- **Task 7** (`extproc/processor.go` ADR-0171 state machine) — depends on Task 4 (`*ProcessorClient` API) + Task 2 (filter struct).
- **Task 8** (`extproc/check.go` ADR-0172 apply + emit + dispatch helpers) — depends on Task 6 (json codec for http-mode path) + Task 7 (state machine constants + helpers).
- **Task 9** (`extproc/attributes.go` envelope builder) — depends on Task 5 (encoder-side ADR-0174 accessors) + Task 2 (filter struct).
- **Task 10** (per-route 5th-canonical resolve + cache-on-first-use + 9-counter filterStats wiring ADR-0173) — depends on Task 2 (compiledConfig struct + filterStats struct).
- **Task 11** (`extproc/extproc.go` `buildCompiledConfig` integration — wires everything from Tasks 3-10 into the factory) — depends on Tasks 3, 4, 6, 7, 8, 9, 10 (consumes all prior surfaces); produces a fully-functional `*HTTPFilter` from `New()`.
- **Task 12** (race tests + OnDestroy + bidi-stream cancellation + concurrent decode/encode) — depends on Task 11 (full filter integration).
- **Task 13** (differential fixture 0022 + `test/helpers/extprocgrpc/` — FIRST in-tree bidi-stream gRPC test-helper) — depends on Task 11 (full filter integration); CLOSES three RATIFIED-PENDING-IMPL-TIME pins (§19.P4 stat surface; §19.P7 per-route cache-on-first-use; §19.P8 JSON codec wire-shape).
- **Task 14** (BEHAVIOR_CONTRACT.md 8-edit bundle + DECISIONS.md final + STATE/ROADMAP advance + 24th fuzzer `FuzzExtProcConfigParse`) — depends on Task 13 (the §13.2 stat-table + §13.3 equivalence-matrix row + §19.P4 RATIFICATION all come from Task 13's fixture-harness scrape); the fuzzer at Task 14 (NOT a separate task per the SPEC author sketch) keeps the task count at 15 by combining the lightweight fuzzer with the BEHAVIOR_CONTRACT bundle.
- **Task 15** (REVIEW per `superpowers:requesting-code-review`) — depends on everything; produces `REVIEW.md` + closes the phase-done gate.

**Parallel-dispatch opportunity at Tasks 3+4+5+6** — four agents can run concurrently on independent surfaces. **Sequential bottleneck at Tasks 7→8 + Task 11** — the state machine + check dispatcher + integration are the critical path. **Tasks 12+13+14** are largely sequential but the unit-test polishing within Task 11 can be parallel-dispatched per Group 1..11.

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-19.1-http-filter-ext-proc-headers-impl \
                 -b phase-19.1-http-filter-ext-proc-headers-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-19.1-http-filter-ext-proc-headers-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-19.1-http-filter-ext-proc-headers-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -6` shows the 19.1-PLAN.md squash commit + its SHA-fill follow-up at the head, with the 19-SPEC.md squash commit `9cc1458` + its SHA-fill follow-up `9975f5d` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `176` (ADR-0176 — the highest ADR anchored as of master tip per the phase-19 parent SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0167' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0167 §Context already at parent SPEC commit per ADR-0044). Same for ADR-0168 through ADR-0175. `grep -cE '^## ADR-0176' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0176 FULL body at parent SPEC commit). `grep -nE '^## ADR-0177' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0177 stays unconsumed at 19.1 IMPL under D12 hypothesis).
6. **NO ADR-0125 §(xiv) amendment.** `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0 matches — phase 19 lands NO ADR-0125 amendment (ADR-0173 records the explicit no-amendment 5th-canonical-REUSE decision; SECOND CONSECUTIVE §9 row after phase 18 to REUSE). If `(xiv)` returns ≥1, investigate before proceeding.
7. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/SPEC.md` returns `9cc1458` (or descendant). If different, re-read SPEC.
8. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
9. **Pristine tree.** `git status --porcelain` returns empty.
10. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
11. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[01])'` returns every fixture 0000–0021 PASS — the 22 pre-existing fixtures are the regression baseline. Phase 19.1 adds the 23rd (`0022-http-ext-proc-grpc` per Task 13).
12. **Pre-existing fuzzers run clean at 30s.** The 23 fuzzers from phases 02–18.2 run clean. Phase 19.1 adds the 24th (`FuzzExtProcConfigParse` per Task 14).
13. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
14. **`google.golang.org/grpc` v1.70.0 reachable.** `go list -m google.golang.org/grpc` returns `google.golang.org/grpc v1.70.0` (DIRECT dep from phase-18.2 per ADR-0158 — no promotion needed at 19.1). `go doc google.golang.org/grpc/ClientStream | head -10` returns the `ClientStream` interface signature with `Send/Recv/CloseSend` (the bidi-stream surface).
15. **`envoy.service.ext_proc.v3` proto package reachable.** `go doc github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3 ExternalProcessorClient | head -10` returns the `ExternalProcessorClient` interface without an `import path failed` error; the `Process` bidi-stream RPC method is visible. Same for `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3 ExternalProcessor` (the filter config). If any fails, `go mod download`.
16. **Pre-existing `internal/filter/http/extproc/` directory does NOT exist.** `test ! -d internal/filter/http/extproc && echo "ok: extproc absent"` returns success.
17. **Pre-existing `test/helpers/extprocgrpc/` directory does NOT exist + `internal/grpcclient/processor_client.go` does NOT exist.** `test ! -d test/helpers/extprocgrpc && test ! -f internal/grpcclient/processor_client.go && echo "ok: extprocgrpc + processor_client.go absent"` returns success.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0167..ADR-0175 §Context drafts are at the parent SPEC commit `9cc1458`; ADR-0176 full body is at the same commit; ADR-0177 is CONDITIONAL (PLAN hypothesis per D12: it does NOT fire at 19.1 IMPL). The PROGRESS preamble ANTICIPATES the 8 ADRs (each with its Lands-in-Task anchor reproduced from this PLAN's per-ADR table) and records the 14 planner-time decisions.

**Precondition:** worktree exists at `phase-19.1-http-filter-ext-proc-headers-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions report green.
**Artifact:** `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` with: (a) Preamble summarizing the 17-precondition verification (verbatim command outputs captured); (b) the 8-ADR table from `## ADRs introduced by this plan` reproduced verbatim; (c) the 14 planner-time decisions reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 1: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: `internal/filter/http/extproc/` skeleton — `doc.go` + `extproc.go` (filter type + factory stub + filterStats + compiledConfig struct + boot-registration) — ADR-0167 + ADR-0168 §Decision (struct + dispatch sketch)

**Files:**
- Create: `internal/filter/http/extproc/doc.go` (~50 LoC)
- Create: `internal/filter/http/extproc/extproc.go` (~400-600 LoC; SKELETON ONLY at this task — the `buildCompiledConfig` body is Task 11; `DecodeHeaders`/`EncodeHeaders` bodies are completed at Tasks 7-10 + integration at Task 11; this task lands the COMPILEABLE skeleton with type + factory stub + struct shapes + boot-registration)
- Modify: `cmd/envoy-go/main.go` (~+1 LoC + +1 import)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0167 + ADR-0168 §Decision draft text — the §Consequences for ADR-0168 land at Task 11 once `buildCompiledConfig` is wired)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 2 entry)

This task lands the package skeleton + the boot-registration + ADR-0167 (the package shape) + ADR-0168 §Decision draft (the `compiledConfig` struct shape + the dispatch sketch). The factory returns `nil, errors.New("ext_proc: factory under construction; lands at Task 11")` for now; compile-time interface assertions (`var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` etc.) pass; boot-registration is alphabetical-between `extauthz` and `fault`. The `extproc_test.go` Group 1 (factory parse paths) lands stubs that will be expanded at Tasks 4-11.

**Precondition:** Task 1 complete; preconditions 16 + 17 verified ABSENT.
**Artifact:** `internal/filter/http/extproc/{doc.go, extproc.go}` (compileable skeleton); `cmd/envoy-go/main.go` (boot-registration added); `docs/envoy-go/DECISIONS.md` (ADR-0167 + ADR-0168 §Decision draft text); `go build ./...` clean; `go vet ./...` clean.
**Acceptance:** `go build ./...` clean; `go vet ./...` clean; `golangci-lint run ./internal/filter/http/extproc/...` clean; compile-time `var _` assertions pass; `grep -nE 'extproc.TypeURL' cmd/envoy-go/main.go` returns the boot-registration line between `extauthz` and `fault`; `grep -cE '^## ADR-0167' docs/envoy-go/DECISIONS.md` returns `1` AND the §Decision body is non-empty (`grep -A2 '^## ADR-0167' docs/envoy-go/DECISIONS.md | grep -c 'Decision'` ≥ 1).

- [ ] **Step 1: Author `internal/filter/http/extproc/doc.go`** — package doc per the File-structure table row above (~50 LoC).

- [ ] **Step 2: Author `internal/filter/http/extproc/extproc.go` SKELETON** — `package extproc` + imports + `const TypeURL = ...` + `type filter struct { ... }` + `type compiledConfig struct { ... }` (full shape per SPEC §6.2) + `type filterStats struct { streamsStarted, streamMsgsSent, ... *stats.Counter }` (9 counters) + `type resolvedProcessingMode struct { ... }` (parsed-and-validated form of `*extprocv3.ProcessingMode`) + `type resolvedMutationRules struct { ... }` (placeholder; populated at Task 11) + `type resolvedForwardRules struct { ... }` (placeholder; populated at Task 11) + stub `func New(cfg *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.HTTPFilter, error) { return nil, errors.New("ext_proc: factory under construction; lands at Task 11") }` + stub `func (f *filter) DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus { return Continue }` (etc. for `DecodeData/DecodeTrailers/EncodeHeaders/EncodeData/EncodeTrailers/OnDestroy` — pass-through Continue / noop) + the three `var _` compile-time assertions + `func newFilterStats(ctx envoyhttp.FactoryCtx, statPrefix string) *filterStats` stub returning a struct with all 9 counters registered.

- [ ] **Step 3: Verify `go build ./...` + `go vet ./...` clean** — the skeleton must compile + pass vet.

- [ ] **Step 4: Modify `cmd/envoy-go/main.go`** — add the import `extproc "github.com/esalaine/envoy-go/internal/filter/http/extproc"` (alphabetical) + the `httpReg.Register(extproc.TypeURL, extproc.New)` line between line `httpReg.Register(extauthz.TypeURL, extauthz.New)` and `httpReg.Register(fault.TypeURL, fault.New)` per ADR-0100 §2.2.

- [ ] **Step 5: Author ADR-0167 §Decision + §Consequences body in DECISIONS.md** — EXTENDS the existing §Context draft at the parent SPEC commit with the full §Decision body covering: package directory + Go-package identifier (`extproc`); BOTH-DECODE-AND-ENCODE `HTTPFilter` shape; 9-counter `filterStats` registered unconditionally at `New()` time; boot-registration alphabetical between `extauthz` and `fault`; multi-stage `SendLocalReply` mechanism (request_headers + response_headers stages in 19.1; body stages at 19.2 — FIRST §9 row whose deny-path can fire at multiple stages); the TWELFTH §9 row framing; FIRST-cross-phase-consumer-of-ADR-0158/0165/0166 framing; the bidi-stream-framework-lift framing. §Consequences body covers: cross-phase reuse intent (ADR-0167 is the package layout reference for ALL future §9 rows that span decode + encode); the 9-counter STRUCTURALLY-UNREACHABLE-counter discipline (mirrors phase-18.2 ADR-0163); the boot-registration alphabetical convention reaffirmation.

- [ ] **Step 6: Author ADR-0168 §Decision DRAFT body in DECISIONS.md** — EXTENDS the existing §Context draft with the §Decision text covering: the `grpc_service`-vs-`http_service` mutually-exclusive top-level field dispatch (NOT a proto oneof per parent §5.P1); the `processorClient` interface (both arms produce it from config-load time); the http_service proto-constraint (PARSE-REJECT body/trailer modes when http_service is set per the proto's `ExtProcHttpService.http_service` doc-comment); body-mode 19.1 PARSE-REJECT (§Decision AMENDED at 19.2 to lift); trailer-mode PARSE-REJECT permanently; STREAMED-only flag PARSE-REJECT permanently (`observability_mode` / `send_body_without_waiting_for_header_response` / `deferred_close_timeout` per parent §5.P10); consumed-vs-deferred field discipline; the error-posture fields (`failure_mode_allow` / `message_timeout` (default 200ms) / `max_message_timeout` (default 0 = override-disabled) / `disable_immediate_response`); GoogleGrpc arm PARSE-REJECT inherited from ADR-0157 §Decision AMENDMENT; `initial_metadata` + `retry_policy` SILENT-IGNORED. §Consequences body is DEFERRED to Task 11 once `buildCompiledConfig` is wired (per the multi-task ADR pattern — Task 2 lands the design intent + struct shape; Task 11 lands the integration completeness + the §Consequences).

- [ ] **Step 7: Append PROGRESS.md Task 2 entry** — record the build output + the boot-registration line shown + ADR-0167 + ADR-0168 §Decision draft hash diffs + `git log -1 --format=%H` for the upcoming Task 2 commit.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/extproc/doc.go \
        internal/filter/http/extproc/extproc.go \
        cmd/envoy-go/main.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 2: extproc package skeleton + boot-registration + ADR-0167 + ADR-0168 §Decision draft

Lands the compileable skeleton for internal/filter/http/extproc/ (doc.go +
extproc.go with filter type + compiledConfig struct + filterStats 9-counter
struct + stub factory + StreamDecoderFilter+StreamEncoderFilter compile-time
assertions). Boot-registration alphabetical between extauthz and fault per
ADR-0100. ADR-0167 §Decision + §Consequences anchored. ADR-0168 §Decision
draft anchored (§Consequences land at Task 11 once buildCompiledConfig is
wired)."
```

---

## Task 3: proto package import verification + go.mod consistency check

**Files:**
- Modify: `go.mod` / `go.sum` (only if Task 2's import wiring surfaced any module-graph cleanup; expected NO net change since `google.golang.org/grpc` v1.70.0 is DIRECT at phase-18.2 + `go-control-plane` v1.32.4 carries the ext_proc proto)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 3 entry)

This task verifies the proto + module surface is reachable for Tasks 4-10. NO new ADR fires; NO code beyond `go mod tidy` if needed. Parallelizable with Tasks 4 + 5 + 6 (independent module verification).

**Precondition:** Task 2 complete; `go build ./...` clean.
**Artifact:** verified `go list -m google.golang.org/grpc` returns `v1.70.0` + verified `go doc github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3 ExternalProcessorClient` returns the expected interface (precondition 14 + 15 re-confirmation); `go mod verify` clean.
**Acceptance:** all 3 verification commands return expected; `go mod tidy` (if needed) produces no net change OR produces a clean diff that compiles + passes tests.

- [ ] **Step 1: Verify proto package reachability** — run `go doc github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3 ExternalProcessorClient | head -10` + `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3 ExternalProcessor | head -10` + `go doc github.com/envoyproxy/go-control-plane/envoy/config/common/mutation_rules/v3 HeaderMutationRules | head -5`. All return non-error output.

- [ ] **Step 2: Verify grpc surface** — `go doc google.golang.org/grpc/ClientStream | head -10` returns `Send/Recv/CloseSend`; `go list -m google.golang.org/grpc` returns `v1.70.0`.

- [ ] **Step 3: Verify protojson surface** — `go doc google.golang.org/protobuf/encoding/protojson MarshalOptions | head -15` returns `UseProtoNames` + `EmitUnpopulated` + `UseEnumNumbers` fields.

- [ ] **Step 4: `go mod tidy`** — if module graph changed during Task 2, run `go mod tidy`; expected NO net change. If the diff is non-empty, append it to PROGRESS.md.

- [ ] **Step 5: Append PROGRESS.md Task 3 entry** — record the 3 verification outputs + `go mod tidy` diff (empty or otherwise).

- [ ] **Step 6: Commit** — only if `go.mod`/`go.sum` changed.

```bash
git add go.mod go.sum docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 3: proto + grpc + protojson reachability verified"
# OR if go.mod/go.sum unchanged:
git add docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 3: proto + grpc + protojson reachability verified (no module-graph change)"
```

---

## Task 4: `internal/grpcclient/processor_client.go` + tests — ADR-0169 §Decision + §Consequences

**Files:**
- Create: `internal/grpcclient/processor_client.go` (~250-400 LoC)
- Create: `internal/grpcclient/processor_client_test.go` (~250-400 LoC; Groups 1+2+3 per SPEC §14.1)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0169 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 4 entry)

This task lands the ADR-0169 bidi-stream wrapper — NEW file alongside the existing `grpcclient.go` per planner-time D11. Completely independent of the extproc package; parallelizable with Tasks 3 + 5 + 6. Production code: `ProcessorClient` struct + `NewProcessorClient` + `Process` + `ProcessStream` interface + `Close`. Test code: Groups 1+2+3.

**Precondition:** Task 2 complete (preconditions met); Task 3 verification confirms grpc v1.70.0 + ext_proc v3 proto reachable.
**Artifact:** `internal/grpcclient/processor_client.go` (NEW); `internal/grpcclient/processor_client_test.go` (NEW); `docs/envoy-go/DECISIONS.md` (ADR-0169 body).
**Acceptance:** `go test -race -count=1 ./internal/grpcclient/...` clean; `go vet ./internal/grpcclient/...` clean; `golangci-lint run ./internal/grpcclient/...` clean; `grep -cE '^## ADR-0169' docs/envoy-go/DECISIONS.md` returns `1` AND the §Decision body covers the SPEC §3.1 sketch + the §Consequences body covers the cross-phase reuse intent.

- [ ] **Step 1: Write Group 1 failing test** — `internal/grpcclient/processor_client_test.go` Group 1 (`NewProcessorClient` happy + PARSE-REJECT paths). Run `go test ./internal/grpcclient/...` → FAIL with `undefined: NewProcessorClient`.

- [ ] **Step 2: Implement `processor_client.go` skeleton** — `type ProcessorClient struct` + `NewProcessorClient` signature + stub body returning `nil, errors.New("not implemented")`. Run Group 1 → still FAIL (test asserts non-nil return on happy path).

- [ ] **Step 3: Implement `NewProcessorClient` body** — calls `d.DialContext(ctx, clusterName)` → wraps with `extprocv3.NewExternalProcessorClient(conn)`. Run Group 1 → PASS.

- [ ] **Step 4: Write Group 2 failing tests** — `(*ProcessorClient).Process` bidi-stream lifecycle + Send/Recv/CloseSend round-trip + mid-stream ctx cancellation + per-message timeout propagation. Run → FAIL with `undefined: Process`.

- [ ] **Step 5: Implement `Process` + `ProcessStream` interface** — opens a bidi-stream via `c.stub.Process(ctx)`; returns a `ProcessStream` interface wrapping the raw `grpc.ClientStream`. Run Group 2 → PASS.

- [ ] **Step 6: Write Group 3 failing tests** — `(*ProcessorClient).Close` idempotency + concurrent close. Run → FAIL.

- [ ] **Step 7: Implement `Close` with `sync.Once`** — Run Group 3 → PASS.

- [ ] **Step 8: Race-detector full sweep** — `go test -race -count=1 ./internal/grpcclient/...` clean (verifies Groups 1+2+3 race-free).

- [ ] **Step 9: Author ADR-0169 §Decision + §Consequences body in DECISIONS.md** — extends the §Context draft at the parent SPEC commit with the full §Decision text covering: bidi-stream wrapper EXTENDING `*Dialer` (NO `Dialer` API changes per ADR-0158 §Consequences); NEW file `processor_client.go` per D11; public surface (`NewProcessorClient` + `Process` + `ProcessStream{Send/Recv/CloseSend}` + `Close`); per-Check timeout becomes per-MESSAGE timeout in `ProcessorClient` (per-message timer applied INSIDE the filter's `dispatchStage` via `context.WithTimeout`, NOT inside `Process` itself); one `*grpc.ClientConn` per (cluster_name, compiledConfig) pair created at config-load time; bidi-stream lifetime per HTTP transaction; `*grpc.ClientConn` reused across streams via gRPC's internal multiplexing; leaks-on-exit MVP per D2 + ADR-0158 §Decision (vi). §Consequences: cross-phase-reusable for any future bidi-stream gRPC filter (FIRST cross-phase consumer of ADR-0158 outside phase-18.2 itself; future streaming-access-log filter would reuse the same wrapper pattern); the per-wrapper-per-file convention going forward (the existing `grpcclient.go`'s `Dialer` + `AuthClient` co-location is grandfathered; `processor_client.go` establishes the new file-per-wrapper pattern per D11).

- [ ] **Step 10: Append PROGRESS.md Task 4 entry** — record the test outputs (PASS counts; race-clean) + ADR-0169 hash diff.

- [ ] **Step 11: Commit**

```bash
git add internal/grpcclient/processor_client.go \
        internal/grpcclient/processor_client_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 4: internal/grpcclient/processor_client.go + ADR-0169

ADR-0169 bidi-stream wrapper EXTENDING *Dialer. NEW file alongside existing
grpcclient.go per planner-time D11. Public surface: NewProcessorClient +
Process + ProcessStream interface + Close. Groups 1+2+3 unit tests green
under -race. FIRST cross-phase consumer of ADR-0158 outside phase-18.2."
```

---

## Task 5: `internal/filter/http/callbacks.go` + `chain.go` + `chain_test.go` extension — ADR-0174 §Decision + §Consequences (encoder-side callback symmetry)

**Files:**
- Modify: `internal/filter/http/callbacks.go` (~+50-100 LoC: 6 new methods on `EncoderFilterCallbacks`)
- Modify: `internal/filter/http/chain.go` (~+50-80 LoC: 6 new `*encoderCB` reader methods consuming the EXISTING ADR-0165 chain fields)
- Modify: `internal/filter/http/chain_test.go` (~+80-120 LoC: 6 seed-and-read round-trip tests + 6 nil/empty fall-through tests for the encoder-side readers)
- Modify: `docs/envoy-go/DECISIONS.md` (~+120 LoC: ADR-0174 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 5 entry)

This task lands the ADR-0174 symmetric `EncoderFilterCallbacks` extension. **PRE-REQUISITE for Task 9** (`attributes.go`'s encoder-side accessor consumption). Per planner-time D10: 6 methods (NOT 7) — `DownstreamRemoteAddr`/`DownstreamLocalAddr`/`DownstreamTLSServerName`/`DownstreamTLSPeerCertDER`/`DownstreamProtocol`/`ListenerPrincipal`. NO new chain fields, NO new seeding primitives, NO new HCM dispatch wiring — the 6 chain fields ALREADY exist per ADR-0165 + are SET-once at HCM dispatch BEFORE either decode or encode dispatch; the new `*encoderCB` reader methods consume the SAME chain fields verbatim. Completely independent of the extproc package; parallelizable with Tasks 3 + 4 + 6.

**Precondition:** Task 2 complete; existing ADR-0165 6 chain fields verified present at `internal/filter/http/chain.go` (the `tlsPrincipals`/`downstreamRemoteAddr`/`downstreamLocalAddr`/`downstreamTLSServerName`/`downstreamTLSPeerCertDER`/`downstreamProtocol`/`listenerPrincipal` chain field roster).
**Artifact:** `internal/filter/http/callbacks.go` (6 new methods); `internal/filter/http/chain.go` (6 new reader methods); `internal/filter/http/chain_test.go` (12 new tests — 6 seed-and-read + 6 nil/empty); `docs/envoy-go/DECISIONS.md` (ADR-0174 body).
**Acceptance:** `go test -race -count=1 ./internal/filter/http/...` clean; `go vet ./internal/filter/http/...` clean; `golangci-lint run ./internal/filter/http/...` clean; `grep -cE '^## ADR-0174' docs/envoy-go/DECISIONS.md` returns `1` AND the §Decision body covers the 6-method extension + chain-field-reuse + NO-new-plumbing claims + §Consequences body covers cross-phase reuse intent + NO race-introduction rationale.

- [ ] **Step 1: Write Group 12 failing tests** — `internal/filter/http/chain_test.go` Group 12 (6 seed-and-read round-trip + 6 nil/empty fall-through; e.g., `TestEncoderCB_DownstreamRemoteAddr_SeededViaSetDownstreamRemoteAddr_ReturnsSeed` per the existing decoder-side template). **NOTE: SPEC §14.1 names `callbacks_test.go` as the Group 12 location, but the existing ADR-0165 decoder-side template lives in `chain_test.go` (where the `*decoderCB` seed/read round-trip tests are co-located with the chain primitives). The PLAN places Group 12 in `chain_test.go` to match the existing decoder-side template layout — empirically correct departure from the SPEC §14.1 filename.** Run → FAIL with `undefined method DownstreamRemoteAddr on EncoderFilterCallbacks`.

- [ ] **Step 2: Extend `EncoderFilterCallbacks` interface in `callbacks.go`** — add 6 new methods per ADR-0174 §Decision draft + doc-comments citing ADR-0174 + cross-phase reuse intent.

- [ ] **Step 3: Implement 6 new reader methods on `*encoderCB` in `chain.go`** — verbatim mirror of the existing `*decoderCB` reader methods; consume the same chain fields. NO new chain fields added; NO new HCM dispatch wiring.

- [ ] **Step 4: Run Group 12 tests** → PASS. Run `go test -race ./internal/filter/http/...` → race-clean.

- [ ] **Step 5: Run repo-wide build + vet + lint** — `go build ./...` + `go vet ./...` + `golangci-lint run ./...` clean (verifies the EncoderFilterCallbacks interface extension doesn't break any existing implementers — since `*encoderCB` is the SOLE concrete implementation in the production tree, no other package needs updating).

- [ ] **Step 6: Author ADR-0174 §Decision + §Consequences body in DECISIONS.md** — extends the §Context draft at the parent SPEC commit. §Decision: 6 new methods on `EncoderFilterCallbacks` (NOT 7 per D10); chain-field-reuse from ADR-0165 (the 6 chain fields ALREADY exist + are SET-once at HCM dispatch BEFORE either decode or encode dispatch — no new chain plumbing); NO race-introduction (encoder-side reads happen after the SET completes; ADR-0071's chain-ownership invariant continues to apply); the `*encoderCB` struct gains 6 reader methods verbatim mirroring `*decoderCB`. §Consequences: cross-phase-reusable for any future encode-side filter needing socket/TLS/listener state (ext_proc is the FIRST consumer at 19.1; future response-mutating or response-attribute-emitting filters reuse the same accessor surface); the asymmetric `DownstreamPrincipal()` decision (decode-side-only per ADR-0144's framing; if the IMPL at Task 9 surfaces a need for response-side `source.principal`, ADR-0174 §Decision goes from 6 → 7 — but the SPEC §3.3 + D10 hypothesis is 6 sufficient).

- [ ] **Step 7: Append PROGRESS.md Task 5 entry** — record the test outputs (12 new tests PASS; race-clean) + ADR-0174 hash diff.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/callbacks.go \
        internal/filter/http/chain.go \
        internal/filter/http/chain_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 5: ADR-0174 EncoderFilterCallbacks 6-method extension

Symmetric extension of EncoderFilterCallbacks mirroring ADR-0165's
DecoderFilterCallbacks methods. 6 new accessors: DownstreamRemoteAddr,
DownstreamLocalAddr, DownstreamTLSServerName, DownstreamTLSPeerCertDER,
DownstreamProtocol, ListenerPrincipal. Reuses ADR-0165's chain fields
(SET-once at HCM dispatch BEFORE either decode or encode dispatch); NO
new chain plumbing or HCM seeding. Group 12 unit tests + nil/empty
fall-throughs green under -race. PRE-REQUISITE for Task 9 (attributes.go
encoder-side consumption)."
```

---

## Task 6: `internal/filter/http/extproc/json.go` — ADR-0170 §Decision + §Consequences (filter-local JSON codec for http_service mode)

**Files:**
- Create: `internal/filter/http/extproc/json.go` (~150-250 LoC)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Groups 1-2 portions covering `marshalProcessingRequest` round-trip + `unmarshalProcessingResponse` happy path + malformed-JSON failure per D8; ~100-150 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (~+100 LoC: ADR-0170 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 6 entry)

This task lands the ADR-0170 filter-local JSON codec for http_service mode. Independent of Tasks 4 + 5; parallelizable. Production: `marshalProcessingRequest` + `unmarshalProcessingResponse` + `protojson` Marshal/UnmarshalOptions setup per D8. Wire-shape RATIFIED-PENDING-IMPL-TIME — closes at Task 13 fixture-harness scrape.

**Precondition:** Task 2 complete; Task 3 verified `protojson` reachable.
**Artifact:** `internal/filter/http/extproc/json.go` (NEW); unit-test portion in `extproc_test.go`; `docs/envoy-go/DECISIONS.md` (ADR-0170 body).
**Acceptance:** `go test -count=1 ./internal/filter/http/extproc/...` (json portion) green; round-trip test asserts `unmarshalProcessingResponse(marshalProcessingRequest(roundtrip-fixture)) == roundtrip-fixture` for a hand-crafted `*extprocv3.ProcessingResponse`; malformed-JSON test asserts non-nil error; `grep -cE '^## ADR-0170' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write failing tests** — `extproc_test.go` Group 2 portion covering `marshalProcessingRequest` round-trip (hand-crafted ProcessingRequest with `request_headers` populated; assert non-empty JSON output) + `unmarshalProcessingResponse` happy path (hand-crafted JSON; assert parsed `*ProcessingResponse` matches) + malformed-JSON failure (truncated JSON; assert non-nil error). Run → FAIL.

- [ ] **Step 2: Implement `json.go`** — `marshalProcessingRequest` uses `protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false, UseEnumNumbers: false}` per ADR-0170 §Decision; `unmarshalProcessingResponse` uses `protojson.UnmarshalOptions{DiscardUnknown: true}` per D8. ~150-250 LoC.

- [ ] **Step 3: Run tests** → PASS.

- [ ] **Step 4: Author ADR-0170 §Decision + §Consequences body in DECISIONS.md** — extends the §Context draft. §Decision: filter-local at `internal/filter/http/extproc/json.go`; `protojson` MarshalOptions (per D8 + parent §5.P8 hypothesis); UnmarshalOptions (forward-compat via `DiscardUnknown: true`); wire-shape RATIFIED-PENDING-IMPL-TIME at parent §5.P8 — closes at Task 13 fixture-harness scrape (one request/response pair vs reference Envoy v1.37.2); on unmarshal failure per D8: classify as `streamsFailed++` + dispError (fail-loud). §Consequences: filter-local for MVP per the phase-18.1 ADR-0159 (b)-disposition rationale (generalization to `internal/jsoncodec/` deferred to the THIRD consumer trigger; currently no other in-tree consumer of protojson-over-HTTP); the IMPL-time AMENDMENT path at parent §5.P8 RATIFIED-PENDING closure (if Envoy diverges from `protojson` defaults at the wire-shape scrape, ADR-0170 §Decision is AMENDED in-place at Task 13 per ADR-0044).

- [ ] **Step 5: Append PROGRESS.md Task 6 entry** — record the test outputs + ADR-0170 hash diff.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/extproc/json.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 6: extproc/json.go + ADR-0170 (filter-local JSON codec)

ADR-0170 filter-local protojson codec for http_service mode. UseProtoNames
+ EmitUnpopulated:false + UseEnumNumbers:false on Marshal; DiscardUnknown
on Unmarshal. Wire-shape RATIFIED-PENDING-IMPL-TIME — closes at Task 13."
```

---

## Task 7: `internal/filter/http/extproc/processor.go` — ADR-0171 §Decision + §Consequences (header-mode state machine + bidi-stream lifecycle)

**Files:**
- Create: `internal/filter/http/extproc/processor.go` (~250-400 LoC)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Group 7 — ProcessingMode resolution + mode_override re-eval + Group 10 — `OnDestroy` cancellation; ~300-500 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0171 header-mode §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 7 entry)

This task lands the bidi-stream state machine + per-stage dispatcher + `OnDestroy` cancel/CloseSend lifecycle per ADR-0171 (header-mode portion; §Decision AMENDED at 19.2 for body-mode). Depends on Task 4 (`*ProcessorClient` API) + Task 2 (filter struct).

**Precondition:** Tasks 2 + 4 complete; `extproc.go` filter struct + `*ProcessorClient` available.
**Artifact:** `internal/filter/http/extproc/processor.go` (NEW); unit tests Groups 7 + 10; ADR-0171 body.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/...` (Groups 7 + 10) green; `grep -cE '^## ADR-0171' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write Group 7 + 10 failing tests** — Group 7 covers `resolveProcessingMode` (DEFAULT → SEND for headers, SKIP for trailers per parent §5.P9; body-mode != NONE PARSE-REJECT; trailer-mode != SKIP PARSE-REJECT; STREAMED-only flag PARSE-REJECT; `http_service` mode + body-mode != NONE PARSE-REJECT) + mode_override re-eval (header-response paths only; body/trailer-response paths silently ignored — NOT spurious; `allow_mode_override:false` ignores all; `allowed_override_modes` allowlist enforced) + `max_message_timeout` guard (`>= 1ms` enables override; out-of-range → `overrideMessageTimeoutIgnored++`; at most ONCE per stage). Group 10 covers `OnDestroy` cancels in-flight `Recv` + `CloseSend` called once. Run → FAIL.

- [ ] **Step 2: Implement `processor.go`** — `type stage int` + `type action int` + `type resolvedProcessingMode struct{}` (parsed-and-validated form) + `(*filter).openProcessorStream` + `(*filter).dispatchStage` (async goroutine; Send + Recv with per-message timeout via `context.WithTimeout` per D6; stat increments) + `(*filter).completeStage` (calls `applyProcessingResponse`; signals `ContinueDecoding`/`ContinueEncoding` on the resume channel) + `(*filter).handleOverrideMessageTimeout` (D6 cancel-and-rebuild; D9 race discipline) + `(*filter).OnDestroy` (`sync.Once`-guarded `streamCancel + CloseSend` per D9). `resolveProcessingMode` parses + validates.

- [ ] **Step 3: Run tests** → PASS under `-race`.

- [ ] **Step 4: Author ADR-0171 header-mode §Decision + §Consequences body in DECISIONS.md** — extends the §Context draft. §Decision: per-direction `ProcessingMode` state; bidi-stream single-in-flight-message correlation; mid-stream `mode_override` re-eval (header-response paths only per parent §5.P1 RATIFIED-AND-REFINED); `allow_mode_override` + `allowed_override_modes` validation; `max_message_timeout >= 1ms` gates `override_message_timeout` enablement; `override_message_timeout` range check `[1ms, max_message_timeout]`; at most ONCE per stage; per-stage timer-reset via `context.WithTimeout` cancel-and-rebuild per D6; STREAMED-only flag PARSE-REJECT; DEFAULT translates to SEND for headers / SKIP for trailers per parent §5.P9. §Consequences: §Decision AMENDED at 19.2 for body-mode (the `*_body_mode = BUFFERED` PARSE-REJECT lifts; body-stage state-machine extends); the D9 race discipline (NO per-stream mutex; relies on framework sequential decode→encode dispatch + `sync.Once` on `OnDestroy`) is cross-phase-reusable for any future bidi-stream filter.

- [ ] **Step 5: Append PROGRESS.md Task 7 entry** + **Step 6: Commit**

```bash
git add internal/filter/http/extproc/processor.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 7: extproc/processor.go + ADR-0171 header-mode state machine

ADR-0171 header-mode portion: ProcessingMode state machine + mode_override
re-eval (header-response paths only per parent §5.P1) + max_message_timeout
discipline (D6 cancel-and-rebuild) + OnDestroy cancel/CloseSend lifecycle
(D9 sync.Once + sequential decode→encode framework discipline). Groups 7
+ 10 green under -race."
```

---

## Task 8: `internal/filter/http/extproc/check.go` — ADR-0172 §Decision + §Consequences (header-mode portion: `applyProcessingResponse` + `applyHeaderMutation` + `emitImmediateResponse` + transport-helpers)

**Files:**
- Create: `internal/filter/http/extproc/check.go` (~600-900 LoC)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Groups 3 + 4 + 5 + 6 + 9 — `buildGRPCProcessorClient`/`buildHTTPProcessorClient` construction; `applyHeaderMutation` + mutation_rules; `applyProcessingResponse` per-stage; `emitImmediateResponse` + content-type sniff; error-posture; ~600-1000 LoC across the groups)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0172 header-mode §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 8 entry)

This task lands the mode-agnostic per-stage dispatcher + CommonResponse mutation + ImmediateResponse multi-stage deny path per ADR-0172 (header-mode portion). Depends on Task 6 (json codec for http-mode dispatch) + Task 7 (state machine constants).

**Precondition:** Tasks 2 + 6 + 7 complete.
**Artifact:** `internal/filter/http/extproc/check.go` (NEW); Groups 3+4+5+6+9 tests; ADR-0172 header-mode body.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/...` (Groups 3+4+5+6+9) green; `grep -cE '^## ADR-0172' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write Group 3 failing tests** — `buildGRPCProcessorClient` + `buildHTTPProcessorClient` construction paths (mocked `cluster.Manager`). Run → FAIL.

- [ ] **Step 2: Implement `buildGRPCProcessorClient` + `buildHTTPProcessorClient`** — per SPEC §6.5: GoogleGrpc PARSE-REJECT + EnvoyGrpc cluster_name PGV-mirror + cluster-manager lookup + UseH2-false PARSE-REJECT + `grpcclient.NewProcessorClient` construction; HTTP-mode validates `HttpService.server_uri.uri` + constructs `*http.Client` + captures `path_prefix`. Run Group 3 → PASS.

- [ ] **Step 3: Write Group 4 failing tests** — `applyHeaderMutation` + per-header `mutation_rules` gating (allowed → applies; rejected → drops + ONCE-per-stage `spurious_msgs_received++`; mutation_rules unset → proto-default protected set host/:authority/:scheme/:method/x-envoy-* applies; mutation_rules set → SUPERSEDES the proto-default per parent §5.P3). Run → FAIL.

- [ ] **Step 4: Implement `applyHeaderMutation` + `resolveMutationRules`** — iterates `set_headers` + `remove_headers` per the 4-arm `append_action` dispatch per D4; per-header `mutationRules.IsAllowed` check; allowed → `dcb`/`ecb` Set/Add/Del; rejected → drop + flag. Run Group 4 → PASS.

- [ ] **Step 5: Write Group 5 failing tests** — `applyProcessingResponse` per-stage dispatch (request_headers + response_headers; stage-mismatch → spurious + dispError; CONTINUE_AND_REPLACE → spurious + dispError in 19.1 per D7; CONTINUE default action). Run → FAIL.

- [ ] **Step 6: Implement `applyProcessingResponse`** — per SPEC §6.7 sketch: (1) ImmediateResponse → emitImmediateResponse (unless `disable_immediate_response` → silent-drop + spurious++); (2) override_message_timeout → handleOverrideMessageTimeout (timer reset; other fields ignored); (3) mode_override → applied only on header-response stages per parent §5.P1; (4) CommonResponse per stage; (5) applyHeaderMutation; (6) clear_route_cache + route_cache_action precedence per parent §5.P5 (request_headers stage ONLY); (7) CONTINUE_AND_REPLACE → spurious + dispError in 19.1 per D7. Run Group 5 → PASS.

- [ ] **Step 7: Write Group 6 failing tests** — `emitImmediateResponse` for both decode and encode stages + grpc_status content-type sniff translation per parent §5.P2. Run → FAIL.

- [ ] **Step 8: Implement `emitImmediateResponse`** — extracts status/headers/body/grpc_status/details; applies `*HeaderMutation` SET/REMOVE per D4 4-arm dispatch; content-type sniff via `f.requestContentType == "application/grpc"`: if gRPC, `body` → `grpc-message` header + `grpc_status` → `grpc-status` response trailer + content-type `application/grpc`; non-gRPC: `body` → `text/plain` default; calls `f.dcb.SendLocalReply` (decode-stage) or `f.ecb.SendLocalReply` (encode-stage — FIRST §9 row to emit `SendLocalReply` from the encode side at the response_headers stage). Run Group 6 → PASS.

- [ ] **Step 9: Write Group 9 failing tests** — error-posture (`failure_mode_allow:false` + processor unreachable → `SendLocalReply(500)` + streamsFailed++; `failure_mode_allow:true` → Continue + failureModeAllowed++ + streamsFailed++; message_timeout exceeded → same as unreachable; `disable_immediate_response:true` + ImmediateResponse arrival → silent-drop + spurious++). Run → FAIL.

- [ ] **Step 10: Implement `mapTransportError` + failure-mode dispatch** — per SPEC §4 + planner-time D5 (existing SendLocalReply primitive suffices for response-headers-delivered branch via `SendLocalReply(0, ...)` stream-reset signal). Run Group 9 → PASS.

- [ ] **Step 11: Race-detector full sweep** — `go test -race -count=1 ./internal/filter/http/extproc/...` clean.

- [ ] **Step 12: Author ADR-0172 header-mode §Decision + §Consequences body in DECISIONS.md** — extends §Context. §Decision: header_mutation set/remove per direction per stage; `mutation_rules` per-header gating per parent §5.P3 (allowed apply; rejected drop + ONCE-per-stage `spurious_msgs_received++`); built-in protected set host/:authority/:scheme/:method/x-envoy-* applies when unset; mutation_rules SUPERSEDES proto-default; `clear_route_cache` + `route_cache_action` precedence per parent §5.P5 (BOTH set → PARSE-REJECT at compiledConfig; either alone honored; neither → DEFAULT; request_headers stage ONLY — body-stage CommonResponse.clear_route_cache ignored per the proto's "This field is ignored in the response direction"); `ImmediateResponse{status, headers, body, grpc_status, details}` with `*HeaderMutation` SET/REMOVE per parent §5.P2 (distinct from phase-18.2's plain `[]HeaderValueOption`); gRPC-downstream-detection via request `content-type: application/grpc` sniff per parent §5.P2; CONTINUE_AND_REPLACE classified as spurious + dispError in 19.1 per D7. §Consequences: §Decision AMENDED at 19.2 for body_mutation + CONTINUE_AND_REPLACE consumed + body-stage immediate_response; FIRST §9 row to emit `SendLocalReply` from the encode side (response_headers stage immediate_response) per ADR-0075 framework support; the gRPC-downstream-detection content-type sniff discipline is cross-phase-reusable for any future filter whose deny-path needs gRPC-vs-HTTP distinction.

- [ ] **Step 13: Append PROGRESS.md Task 8 entry** + **Step 14: Commit**

```bash
git add internal/filter/http/extproc/check.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 8: extproc/check.go + ADR-0172 header-mode portion

ADR-0172 header-mode portion: applyProcessingResponse + applyHeaderMutation
+ emitImmediateResponse + buildGRPCProcessorClient + buildHTTPProcessorClient
+ failure-mode dispatch. Groups 3+4+5+6+9 green under -race. FIRST §9 row
to emit SendLocalReply from the encode side (response_headers stage)."
```

---

## Task 9: `internal/filter/http/extproc/attributes.go` — attribute envelope builder (consumes Task 5 encoder-side accessors)

**Files:**
- Create: `internal/filter/http/extproc/attributes.go` (~250-400 LoC)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Group 11 — attribute envelope builder with mocked TLS state; ~200-300 LoC)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 9 entry)

This task lands `buildRequestHeadersProcessingRequest` + `buildResponseHeadersProcessingRequest` + `buildAttributeEnvelope` + helpers. CONSUMES Task 5's ADR-0174 encoder-side accessors. NO new ADR fires (the attribute-name mapping is an IMPL-level concern per D3; refinement against reference Envoy v1.37.2 closes at Task 13 fixture-harness scrape per parent §5.P4-class — if the IMPL settle adds a 7th encoder-side method per D10 contingency, ADR-0174 §Decision is AMENDED in-place at Task 9 — but PLAN's strong hypothesis is 6 suffices).

**Precondition:** Tasks 2 + 5 complete; the ADR-0174 6 encoder-side methods landed at Task 5.
**Artifact:** `internal/filter/http/extproc/attributes.go` (NEW); Group 11 tests.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/...` (Group 11) green; the attribute-name → accessor mapping matches the SPEC §6.6 table; mocked TLS state populates `connection.requested_server_name` + `connection.subject_local_certificate`; encoder-side path returns empty `source.principal` per D10 hypothesis.

- [ ] **Step 1: Write Group 11 failing tests** — `buildRequestHeadersProcessingRequest` populates `request_headers` + `attributes` map from `f.dcb` + the configured allowlist; `buildResponseHeadersProcessingRequest` populates `response_headers` + `attributes` from `f.ecb`. Mocked TLS state in `*filter`: `f.dcb.DownstreamTLSServerName()` returns `"example.com"` → `connection.requested_server_name` = `"example.com"`. Empty allowlist → no `attributes` field. Allowlist `["source.address"]` → only that attribute populated. ENCODE-side `source.principal` empty per D10. Run → FAIL with `undefined: buildRequestHeadersProcessingRequest`.

- [ ] **Step 2: Implement `attributes.go`** — `buildRequestHeadersProcessingRequest(f, headers, endStream, allowlist) *extprocv3.ProcessingRequest` per SPEC §6.6; `buildResponseHeadersProcessingRequest` symmetric (uses ADR-0174 encoder-side accessors); `buildAttributeEnvelope(allowlist, addressFn, principalFn, tlsServerNameFn, ...) map[string]*structpb.Value` driven by the SPEC §6.6 table; `lowercaseHeaderMap` + `sourcePrincipalFirstOrEmpty` helpers. Run Group 11 → PASS.

- [ ] **Step 3: Race-detector sweep** — `go test -race ./internal/filter/http/extproc/...` clean.

- [ ] **Step 4: Append PROGRESS.md Task 9 entry** + **Step 5: Commit**

```bash
git add internal/filter/http/extproc/attributes.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 9: extproc/attributes.go envelope builder

Consumes Task 5 ADR-0174 encoder-side accessors for response_attributes
population at response_headers stage. Decoder-side request_attributes
consumes ADR-0165's existing 6 methods + ADR-0144's DownstreamPrincipal.
Group 11 tests green. Attribute-name → accessor mapping per SPEC §6.6
hypothesis — RATIFIED at Task 13 fixture-harness scrape per parent §5.P4."
```

---

## Task 10: Per-route 5th-canonical resolution + cache-on-first-use + 9-counter filterStats wiring — ADR-0173 §Decision + §Consequences

**Files:**
- Extend: `internal/filter/http/extproc/extproc.go` (~+150-250 LoC: per-route resolution + `resolvedPerRoute` struct + cache-on-first-use discipline + `newFilterStats` body wiring all 9 counters + `ExtProcPerRoute` parsing helpers)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Group 8 — per-route 5th-canonical REUSE + cache-on-first-use + 9-counter SHARED-stats; ~300-500 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (~+150 LoC: ADR-0173 §Decision + §Consequences body)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 10 entry)

This task lands ADR-0173 — per-route 5th-canonical REUSE + SHARED-stats + 9-counter stat surface. Depends on Task 2 (compiledConfig + filterStats struct shape established).

**Precondition:** Task 2 complete; `compiledConfig` struct + `filterStats` struct + `newFilterStats` stub exist.
**Artifact:** `internal/filter/http/extproc/extproc.go` (per-route + filterStats wired); Group 8 tests; ADR-0173 body.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/...` (Group 8) green; per-route disabled → `Continue` immediately + zero counter increments; per-route overrides processing_mode + grpc_service consumed; `async_mode`/`request_attributes`/`response_attributes`/`metadata_options`/`grpc_initial_metadata` silent-ignored; cache-on-first-use confirmed (the per-route resolved at DecodeHeaders time stays in effect across `ClearRouteCache` invocations); all 9 counters unconditionally registered at `New()` time; `grep -cE '^## ADR-0173' docs/envoy-go/DECISIONS.md` returns `1`.

- [ ] **Step 1: Write Group 8 failing tests** — per-route `disabled:true` → `Continue` + zero counter increments; per-route `overrides{processing_mode}` → consumed (effective ProcessingMode overrides listener-level); per-route `overrides{grpc_service}` → routes to alternate cluster; per-route silent-ignored fields (per parent §5.P6 — async_mode + request_attributes + response_attributes + metadata_options + grpc_initial_metadata); cache-on-first-use across `ClearRouteCache` invocations; per-route `disabled:false` PARSE-REJECT (PGV `const: true`); empty `ExtProcPerRoute` PARSE-REJECT (override oneof PGV-required); all 9 counters appear in `/stats` output. Run → FAIL.

- [ ] **Step 2: Implement per-route resolution + `resolvedPerRoute` struct** — extends `extproc.go` with `type resolvedPerRoute struct { disabled bool; effectiveProcessingMode *resolvedProcessingMode; processorClient interface{...} }` + `func parseExtProcPerRoute(*extprocv3.ExtProcPerRoute) (*resolvedPerRoute, error)` (validates the oneof + the PGV `const: true` discipline + silent-ignores the `[#not-implemented-hide:]` fields); `(*filter).resolvePerRoute()` calls `f.dcb.RequestRouteConfig()` + caches on `f.perRoute` on first call (subsequent calls return the cached value — cache-on-first-use per parent §5.P7). Run Group 8 (per-route portions) → PASS.

- [ ] **Step 3: Implement `newFilterStats` body** — registers all 9 counters under `http.<HCM_stat_prefix>.ext_proc.*` per parent §5.P4 hypothesis (SN2-reuse — the existing HCM-stat-prefix Prometheus tag-extractor handles this verbatim; NO new SN-flattening rule). All 9 unconditionally registered at `New()` time (mirrors phase-18.2 ADR-0163 `disabled`-counter STRUCTURALLY-UNREACHABLE discipline for scrape-stability). Run Group 8 (stat portions) → PASS.

- [ ] **Step 4: Race-detector sweep** — `go test -race ./internal/filter/http/extproc/...` clean.

- [ ] **Step 5: Author ADR-0173 §Decision + §Consequences body in DECISIONS.md** — extends §Context. §Decision: per-route 5th-canonical REUSE (explicit no-new-canonical decision); NO ADR-0125 amendment paragraph (SECOND CONSECUTIVE §9 family-row after phase 18 to REUSE; the absence of a §(xiv) amendment is itself a recorded decision); SHARED-stats discipline (per-route adjusts processing_mode/grpc_service but spawns no new stateful policy-evaluation surface); `ExtProcOverrides` narrower-override surface (MVP-CONSUMED: processing_mode + grpc_service; SILENT-IGNORED: async_mode + request_attributes + response_attributes (per-route #3/#4 `[#not-implemented-hide:]` — distinct from top-level #5/#6 which ARE consumed) + metadata_options + grpc_initial_metadata); 9-counter stat surface RATIFIED-PENDING-IMPL-TIME (closes at Task 13 fixture-harness scrape); cache-on-first-use per parent §5.P7 (closes at Task 13 mid-stream-ClearRouteCache scenario); PGV wrinkles (`disabled` `const: true` PARSE-REJECTs `disabled:false`; `override` oneof PGV-required PARSE-REJECTs empty `ExtProcPerRoute`). §Consequences: SECOND CONSECUTIVE §9 row to REUSE 5th canonical strengthens ADR-0125's roster-not-monotonic lesson; the SHARED-stats discipline becomes the default for future §9 rows whose per-route override adjusts policy parameters without spawning new stateful surfaces; the 9-counter unconditional-allocation discipline (per ADR-0163 STRUCTURALLY-UNREACHABLE-counter scrape-stability) extends to ext_proc's `disabled` counter (the per-route `disabled:true` path increments NO counters — but the 9 counters are all registered at New() time for stable Prometheus scrape output).

- [ ] **Step 6: Append PROGRESS.md Task 10 entry** + **Step 7: Commit**

```bash
git add internal/filter/http/extproc/extproc.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 10: per-route 5th-canonical + 9-counter stats + ADR-0173

ADR-0173 per-route 5th-canonical REUSE (NO ADR-0125 amendment — SECOND
CONSECUTIVE §9 row after phase 18); SHARED-stats; ExtProcOverrides
narrower-override (MVP-CONSUMED: processing_mode + grpc_service); 9-counter
filterStats unconditionally registered at New() time; cache-on-first-use
per parent §5.P7. Group 8 green."
```

---

## Task 11: `extproc.go` `buildCompiledConfig` integration — wires Tasks 3+4+6+7+8+9+10 into a fully-functional factory + ADR-0168 §Consequences

**Files:**
- Extend: `internal/filter/http/extproc/extproc.go` (~+200-300 LoC: `buildCompiledConfig` full body + `DecodeHeaders`/`EncodeHeaders` full bodies per SPEC §6.3 + §6.4 + `New` factory completion)
- Extend: `internal/filter/http/extproc/extproc_test.go` (Groups 1 + 2 expansion — factory parse paths completing all PARSE-REJECT branches + `compiledConfig` field values post-parse for all configurations; ~400-700 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (~+80 LoC: ADR-0168 §Consequences body completion — the §Decision was drafted at Task 2)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 11 entry)

This task lands the FULL `buildCompiledConfig` body wired against all prior tasks' surfaces; the `New` factory becomes fully functional. ADR-0168 §Consequences anchors at this task (the §Decision text at Task 2 already covers the design intent; the §Consequences cover the integration completeness + downstream-consumer impact).

**Precondition:** Tasks 3 + 4 + 6 + 7 + 8 + 9 + 10 complete (every dependency wired). 
**Artifact:** Full `extproc.go` with functional `New` + `buildCompiledConfig` + `DecodeHeaders` + `EncodeHeaders`; Groups 1+2 expanded; ADR-0168 §Consequences complete.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/...` (Groups 1+2 + carry-forward Groups 3-11) green; `go test -race -count=1 ./...` repo-wide clean; `New(anypb.New(&validConfig))` returns non-nil filter; `New(anypb.New(&invalidConfig))` returns error per the SPEC PARSE-REJECT discipline; `grep -A1 '^## ADR-0168' docs/envoy-go/DECISIONS.md | grep -c 'Consequences'` ≥ 1.

- [ ] **Step 1: Write Group 1 + 2 expansion failing tests** — every PARSE-REJECT branch from SPEC §15 item 2 covered: both-set OR neither-set service → PARSE-REJECT; body-mode != NONE PARSE-REJECT in 19.1; trailer-mode != SKIP PARSE-REJECT; STREAMED-only flag PARSE-REJECT; GoogleGrpc PARSE-REJECT; route_cache_action + disable_clear_route_cache mutual-exclusion PARSE-REJECT; HTTP-service body-mode PARSE-REJECT per proto constraint; EnvoyGrpc cluster_name empty PARSE-REJECT; unknown cluster PARSE-REJECT; UseH2:false cluster PARSE-REJECT. Group 2 covers `compiledConfig` field values post-parse for both gRPC + HTTP arms. Run → FAIL or NOT-ALL-PASS (some pass from Task 2 stubs; the rest depend on `buildCompiledConfig` body).

- [ ] **Step 2: Implement `buildCompiledConfig` full body** — per SPEC §6.5 sketch's 10-step body (mutual-exclusion check; build transport client via Task 8's `buildGRPCProcessorClient` / `buildHTTPProcessorClient`; `resolveProcessingMode` via Task 7's resolver; error-posture fields with defaults; STREAMED-only flag PARSE-REJECT; route_cache_action mutual-exclusion + `disable_clear_route_cache` → RETAIN translation per parent §5.P5; `resolveMutationRules` + `resolveForwardRules`; allowed_override_modes; attribute envelopes; statPrefix + `newFilterStats` call via Task 10). Wires every Task's deliverable.

- [ ] **Step 3: Implement full `DecodeHeaders` + `EncodeHeaders` bodies** — replace Task 2's pass-through stubs with the full SPEC §6.3 + §6.4 bodies; `DecodeHeaders` resolves per-route (Task 10) → checks `request_header_mode` (Task 7) → builds request_headers ProcessingRequest (Task 9) → opens stream (Task 7) → dispatches stage (Task 7); `EncodeHeaders` analogously. `New` factory becomes fully functional — calls `buildCompiledConfig` + returns a `*filter` with the embedded `compiledConfig`.

- [ ] **Step 4: Run Groups 1+2** → PASS. Run full Groups 3-11 → PASS (carry-forward all prior tasks' tests).

- [ ] **Step 5: Repo-wide race-detector sweep** — `go test -race -count=1 ./...` clean.

- [ ] **Step 6: Author ADR-0168 §Consequences body in DECISIONS.md** — the §Decision text at Task 2 already covers design intent; §Consequences now covers: the `buildCompiledConfig` 10-step pipeline as the operational sequence; the closure-capture-vs-struct-field discipline (`include_*` gates, `pack_as_bytes`, etc. for any future filter-specific config can ride in the closure rather than `compiledConfig` fields — mirrors phase-18.2 ADR-0157 §Decision); the §Decision AMENDMENT cross-reference at 19.2 (the body-mode PARSE-REJECT lift at 19.2 IMPL replaces the 19.1 `resolveProcessingMode` body-mode PARSE-REJECT with body-mode dispatch; ADR-0172 body-mode AMENDMENT extends the consumption surface).

- [ ] **Step 7: Append PROGRESS.md Task 11 entry** + **Step 8: Commit**

```bash
git add internal/filter/http/extproc/extproc.go \
        internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 11: buildCompiledConfig integration + ADR-0168 §Consequences

Wires Tasks 3+4+6+7+8+9+10 into the New factory. Full DecodeHeaders +
EncodeHeaders bodies. ADR-0168 §Consequences anchored. Groups 1+2 expanded
to cover all PARSE-REJECT branches per SPEC §15 item 2. All Groups 1-11
green under -race; repo-wide -race clean."
```

---

## Task 12: Race tests — `OnDestroy` cancellation + bidi-stream half-close lifecycle + concurrent decode/encode dispatch

**Files:**
- Extend: `internal/filter/http/extproc/extproc_test.go` (Group 10 expansion + NEW dedicated race tests; ~300-500 LoC)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 12 entry)

This task hardens the race-detector surface per D9 + SPEC §14.2. Production code UNCHANGED (race issues surfaced at IMPL fixed in-place per `superpowers:systematic-debugging`); the task adds focused race tests covering: `OnDestroy`-during-in-flight-Send/Recv; concurrent encode-stage dispatch alongside decode-stage dispatch on the SAME bidi-stream (the framework dispatches sequentially decode→encode per HTTP transaction; the race-test confirms the sequential dispatch hold + asserts no race); mode_override mid-stream race (the `f.activeProcessingMode` mutation on the request_headers recv goroutine; READ on response_headers dispatch); bidi-stream `Send` blocking on `Recv` returning (gRPC semantics — only one Send + one Recv per stream).

**Precondition:** Task 11 complete (full filter functional).
**Artifact:** `extproc_test.go` Group 10 expansion + new tests `TestOnDestroy_CancelsInFlightProcessorStream` + `TestSequentialDecodeEncodeDispatchNoRace` + `TestModeOverrideRaceClean` + `TestBidiStreamSendRecvDiscipline`.
**Acceptance:** `go test -race -count=1 ./internal/filter/http/extproc/... ./internal/grpcclient/... ./internal/filter/http/...` clean over 10 iterations (`-count=10` if reproducible flakes suspected).

- [ ] **Step 1: `TestOnDestroy_CancelsInFlightProcessorStream`** — spawn a slow processor (server intentionally sleeps before responding); issue request → DecodeHeaders dispatches → `OnDestroy` fires while `Recv` is blocked → assert `Recv` returns `context.Canceled` promptly + `CloseSend` called exactly once (verified via the test-server's tracked CloseSend count).

- [ ] **Step 2: `TestSequentialDecodeEncodeDispatchNoRace`** — issue request through full filter integration (HCM dispatch) → assert request_headers stage completes BEFORE response_headers stage starts (verified by goroutine-ID capture in dispatch goroutines via `runtime.Stack`); race-detector clean over 100 iterations.

- [ ] **Step 3: `TestModeOverrideRaceClean`** — request_headers ProcessingResponse arrives with `mode_override{response_header_mode: SKIP}`; response_headers stage skipped (no ProcessingRequest sent); race-detector clean.

- [ ] **Step 4: `TestBidiStreamSendRecvDiscipline`** — verifies only one goroutine calls Send + only one calls Recv per stage's dispatch (no concurrent Send-vs-Send OR Recv-vs-Recv on the same stream); race-detector clean.

- [ ] **Step 5: Iterate `-race` over `-count=10`** — `go test -race -count=10 ./internal/filter/http/extproc/...` clean; if any flake surfaces, fix in production code per `superpowers:systematic-debugging` + re-test.

- [ ] **Step 6: Append PROGRESS.md Task 12 entry** + **Step 7: Commit**

```bash
git add internal/filter/http/extproc/extproc_test.go \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 12: race tests for OnDestroy + bidi cancellation + sequential dispatch

Per planner-time D9 — confirms gRPC ClientStream concurrent Send+Recv
discipline + framework sequential decode→encode dispatch + sync.Once on
OnDestroy. -race clean over 10 iterations."
```

---

## Task 13: Differential fixture `0022-http-ext-proc-grpc` + `test/helpers/extprocgrpc/` + RATIFIED-PENDING-IMPL-TIME pin closures (§19.P4 + §19.P7 + §19.P8)

**Files:**
- Create: `test/helpers/extprocgrpc/doc.go` (~25 LoC)
- Create: `test/helpers/extprocgrpc/extprocgrpc.go` (~200-300 LoC; FIRST in-tree bidi-stream gRPC server)
- Create: `test/helpers/extprocgrpc/extprocgrpc_test.go` (~120-180 LoC)
- Create: `test/fixtures/0022-http-ext-proc-grpc/envoy.yaml` (~250 LoC)
- Create: `test/fixtures/0022-http-ext-proc-grpc/envoy-go.yaml` (~250 LoC)
- Create: `test/fixtures/0022-http-ext-proc-grpc/expectations.yaml` (~90 LoC)
- Create: `test/fixtures/0022-http-ext-proc-grpc/README.md` (~150 LoC)
- Create: `test/fixtures/0022-http-ext-proc-grpc/inputs/driver.go` (~400 LoC)
- Modify: `test/differential/fixture/fixture.go` (~+15 LoC: `HTTPExtProcGRPC BackendKind = 19`)
- Modify: `test/differential/runner_test.go` (~+12 LoC: blank import + switch-case)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 13 entry — includes the verbatim scrape closure for §19.P4 + §19.P7 + §19.P8)

This task lands the differential fixture + the FIRST in-tree bidi-stream gRPC test-helper. It CLOSES the three RATIFIED-PENDING-IMPL-TIME pins via fixture-harness empirical scrape against reference Envoy v1.37.2. Per planner-time D13: three-listener topology (l_test_a/b/c). Per planner-time D1: `:path`-discriminator scriptable per-stage response sequence.

**Precondition:** Task 11 complete (full filter); Task 12 race-clean (the fixture exercises live bidi-stream cancellation).
**Artifact:** `test/helpers/extprocgrpc/` directory (NEW); `test/fixtures/0022-http-ext-proc-grpc/` directory (NEW); `test/differential/{fixture,runner_test}.go` extended; 8 scenarios green under `go test ./test/differential/ -run Test.*0022`; three RATIFIED-PENDING-IMPL-TIME pin closures captured in PROGRESS.md.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0022'` returns 8/8 PASS; `go test ./test/helpers/extprocgrpc/...` PASS; counter-delta scrape against reference Envoy v1.37.2 matches the 9-counter hypothesis (closes §19.P4 RATIFIED OR triggers in-place ADR-0173 §Decision AMENDMENT); mid-stream `ClearRouteCache` scenario asserts cache-on-first-use behavior matches reference (closes §19.P7 RATIFIED OR triggers in-place ADR-0173 §Decision AMENDMENT); HTTP-mode scenario 6 JSON wire-shape scrape asserts `protojson` defaults match reference (closes §19.P8 RATIFIED OR triggers in-place ADR-0170 §Decision AMENDMENT).

- [ ] **Step 1: Implement `test/helpers/extprocgrpc/`** — per the File-structure-table row. `Server` struct + `New(t)` + `Addr()` + `Script(discriminator, responses...)` + `Process(stream)` server method (Recv-loops + per-discriminator script counter + per-stage Send) + `Stop()` graceful close. Tests cover Server lifecycle + scripted sequence + bidi-stream half-close (client `CloseSend` after first response → server returns nil cleanly) + ImmediateResponse arm in script terminates stream + concurrent client + race-clean.

- [ ] **Step 2: Author `envoy.yaml` + `envoy-go.yaml`** — three-listener topology per D13; per-listener `ExternalProcessor` config; routes for scenarios 1-8; per-route TPFC for scenarios 7 (`/disabled`) + 8 (`/override`); cluster `c_ext_proc` with `http2_protocol_options: {}` (mandatory for gRPC framing). The 8 scenarios per SPEC §7.1 table.

- [ ] **Step 3: Author `inputs/driver.go`** — `runScenario1..runScenario8` functions per SPEC §7.1; `setupProcessorGRPC` helper spawns `extprocgrpc.New(t)` + scripts the per-scenario `ProcessingResponse` sequences before each scenario; counter-delta scrape helpers mirror phase-18.2. Per SPEC §15 item 9 + §15 item 10: each scenario asserts byte-exact body + status equivalence + counter-delta equivalence + processor-server received-ProcessingRequest content assertions (including the attribute envelope content per the SPEC §6.6 hypothesis-mapping table).

- [ ] **Step 4: Implement the THREE RATIFIED-PENDING-IMPL-TIME pin closures**:
  - **§19.P4 (9-counter stat surface)**: post-scenario-1 — scrape `/stats/prometheus` from BOTH reference Envoy AND envoy-go; assert the counter NAMES match the 9-hypothesis verbatim under `http.<HCM_stat_prefix>.ext_proc.*`. If divergent, ADR-0173 §Decision AMENDED in-place (counter renamed/added/removed); record the AMENDMENT in PROGRESS.md.
  - **§19.P7 (cache-on-first-use per-route after ClearRouteCache)**: scenario 8 (`/override`) carries TWO assertions — the per-route processing_mode override assertion (the primary scenario contract) AND the cache-on-first-use assertion (the §19.P7 closure). Processor script returns `CommonResponse{clear_route_cache: true}` at request_headers stage; the fixture wires a route-config that would re-select a DIFFERENT per-route on the new path lookup; assert the per-route stays at the DecodeHeaders-time-resolved value (cache-on-first-use). If reference Envoy diverges (re-resolves at each stage), ADR-0173 §Decision AMENDED.
  - **§19.P8 (JSON codec wire-shape vs `protojson` defaults)**: scenario 6 (HTTP-mode) — capture one ProcessingRequest POST body + one ProcessingResponse response body from both reference Envoy AND envoy-go; assert byte-equivalent rendering. If divergent (e.g., reference Envoy uses lowerCamelCase or emits empty fields), ADR-0170 §Decision AMENDED in-place (e.g., flip `UseProtoNames: false` or `EmitUnpopulated: true`).

- [ ] **Step 5: Run differential** — `go test -count=1 ./test/differential/ -run 'Test.*0022'` → 8/8 PASS. The harness asserts byte-equivalence + counter-delta equivalence. If any assertion fails, EITHER fix the production code (debug per `superpowers:systematic-debugging`) OR amend the ADR in-place (per the SPEC-time AMENDMENT-allowed surface for the 3 RATIFIED-PENDING pins).

- [ ] **Step 6: Run full regression** — `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[012])'` → 23/23 PASS (0000-0022; 0022 NEW).

- [ ] **Step 7: Append PROGRESS.md Task 13 entry** — record verbatim the 3 pin closure outputs (counter scrape match; cache-on-first-use scenario assertion; JSON wire-shape scrape match). Document any in-place ADR AMENDMENTS landed.

- [ ] **Step 8: Commit**

```bash
git add test/helpers/extprocgrpc/ \
        test/fixtures/0022-http-ext-proc-grpc/ \
        test/differential/fixture/fixture.go \
        test/differential/runner_test.go \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
# Plus DECISIONS.md if any ADR AMENDMENT landed at the pin closures:
git add docs/envoy-go/DECISIONS.md  # if amended
git commit -m "phase 19.1 Task 13: fixture 0022 + extprocgrpc helper + RATIFIED-PENDING pins closed

Differential fixture 0022-http-ext-proc-grpc with 8 scenarios across BOTH
service modes + per-route discipline + failure_mode_allow + mode_override
+ immediate_response. FIRST in-tree bidi-stream gRPC test-helper at
test/helpers/extprocgrpc/ (extends the phase-18.2 unary extauthzgrpc
pattern to bidi). Three RATIFIED-PENDING-IMPL-TIME pin closures:
§19.P4 (9-counter stat surface RATIFIED at fixture-harness scrape);
§19.P7 (cache-on-first-use per-route RATIFIED at mid-stream
ClearRouteCache scenario); §19.P8 (JSON codec wire-shape RATIFIED at
http_service scenario scrape). 23/23 differential fixtures green
(0000-0022; 0022 NEW)."
```

---

## Task 14: 24th fuzzer `FuzzExtProcConfigParse` + BEHAVIOR_CONTRACT.md 8-edit bundle + DECISIONS.md final + STATE/ROADMAP advance

**Files:**
- Create: `internal/filter/http/extproc/fuzz_test.go` (~100 LoC; 24th fuzzer per SPEC §7.3)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (~+400 LoC; 8-edit bundle per SPEC §13)
- Modify: `docs/envoy-go/DECISIONS.md` (final-state verification; any remaining ADR §Consequences cleanup)
- Modify: `docs/envoy-go/ROADMAP.md` (~+1 net: row 19.1 status `in-progress → done` + `last-touched` date-stamp)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place per `BOOTSTRAP_PROMPT.md` §5)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 14 entry)

This task lands the 24th fuzzer (combined with the doc bundle for task-count economy) + the BEHAVIOR_CONTRACT 8-edit bundle (per SPEC §13.1 through §13.8) + the STATE/ROADMAP advance. The parent row 19 STAYS `in-progress` per parent SPEC §8 (it closes at 19.2's phase-done AT THE SAME commit per the parent-rollup discipline).

**Precondition:** Tasks 1-13 complete; all 5 production gates green (build/vet/lint/race/differential).
**Artifact:** `internal/filter/http/extproc/fuzz_test.go` (NEW); BEHAVIOR_CONTRACT.md (8 patches landed); ROADMAP.md (row 19.1 done); STATE.md (advanced).
**Acceptance:** 24th fuzzer runs clean at 30s under ADR-0018 budget; existing 23 fuzzers re-run clean; `tools/check_behavior_contract.sh` (or analog) green; `grep -nE '### envoy.filters.http.ext_proc' docs/envoy-go/BEHAVIOR_CONTRACT.md` returns 1 match; ROADMAP row 19.1 reads `done`; STATE.md lifecycle reads `phase 19.1 done; phase 19 parent in-progress; phase 19.2 SPEC pending`; all 6 phase-done gates per BOOTSTRAP_PROMPT §7.5 green.

- [ ] **Step 1: Author `fuzz_test.go`** — 24th fuzzer `FuzzExtProcConfigParse` per SPEC §7.3. Seeds 8-12 valid `ExternalProcessor` encodings + PARSE-REJECT-triggering variants. Run `go test -fuzz=FuzzExtProcConfigParse -fuzztime=30s ./internal/filter/http/extproc/...` → clean (no panic, no `Fuzz` failure).

- [ ] **Step 2: Re-run existing fuzzers** — `go test -fuzz=. -fuzztime=30s ./...` (one fuzzer per package; the 23 from phases 02-18.2) → clean.

- [ ] **Step 3: BEHAVIOR_CONTRACT.md 8-edit bundle**:
  - **§13.1**: `## HTTP filter chain` → NEW `### envoy.filters.http.ext_proc` subsection (~120 LoC). See SPEC §13.1 content.
  - **§13.2**: `## Stat-name mapping` → 77 → ~86 names (~30 LoC). Add 9 new rows under `http.<HCM_stat_prefix>.ext_proc.*` per the Task 13 §19.P4 closure.
  - **§13.3**: `## Equivalence Matrix` → NEW row for fixture `0022-http-ext-proc-grpc` (~5 LoC).
  - **§13.4**: NEW `### Phase 19.1 forward-pointer notes` subsection (~60 LoC) under `## Forward-pointer notes` covering the SPEC §8 18-item deferral list + the 1 closure of phase-18.2 forward-pointer (encode-side callback symmetry via ADR-0174).
  - **§13.5**: `## HTTPFilterCallbacks` AMENDMENT (~30 LoC) per ADR-0174 — documents the 6 new methods landed on `EncoderFilterCallbacks`.
  - **§13.6**: `## Per-route canonical patterns cross-reference` UNCHANGED — no net edit; ADR-0173 records the no-amendment 5th-canonical-REUSE.
  - **§13.7**: `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella EXTENSION (~50 LoC) — documents the NEW `*ProcessorClient` bidi-stream wrapper (ADR-0169).
  - **§13.8**: NEW `### JSON codec note` (~20 LoC) lighter-touch reference under §13.7 umbrella.

- [ ] **Step 4: Verify BEHAVIOR_CONTRACT consistency** — `tools/check_behavior_contract.sh` (or analog) green; the 9-counter stat-table row count matches the production filterStats; the equivalence-matrix row for 0022 is grep-visible.

- [ ] **Step 5: ROADMAP.md update** — row 19.1 status `in-progress → done`; `last-touched` date-stamp updated; summary sharpening per the File-structure-table row above (post-impl counts: 15-task PLAN-confirmed; final 8-ADR roster anchored). Row 19 (parent) UNCHANGED (`in-progress`); row 19.2 UNCHANGED (`planned`).

- [ ] **Step 6: STATE.md rewrite** — `active-phase: 19.2-http-filter-ext-proc-body` (the next lifecycle target); `lifecycle-state: phase 19.1 done; phase 19 parent in-progress; phase 19.2 SPEC pending`; `next-skill: superpowers:brainstorming` (or `superpowers:writing-specs` if the 19.2 SPEC session skips brainstorm — per the 18.2 precedent the SPEC author may choose); `last-commit: <Task 14 commit SHA — TBD before squash; SHA-fill follow-up after squash-merge>`; `next-free ADR: ADR-0177` (assuming D12 hypothesis held); `last-updated: <impl-date>`.

- [ ] **Step 7: Append PROGRESS.md Task 14 entry** — record the fuzzer outputs + the BEHAVIOR_CONTRACT diff summary + the STATE/ROADMAP advance verbatim.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/extproc/fuzz_test.go \
        docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/STATE.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 14: 24th fuzzer + BEHAVIOR_CONTRACT bundle + STATE/ROADMAP advance

24th fuzzer FuzzExtProcConfigParse clean at 30s (existing 23 fuzzers
re-run clean). BEHAVIOR_CONTRACT 8-edit bundle landed per SPEC §13.
ROADMAP row 19.1 → done (parent row 19 STAYS in-progress per §8 rollup);
STATE advanced to 19.2-pending lifecycle state. Next-free ADR-0177
(D12 hypothesis HELD — no impl-time-unanticipated ADRs fired)."
```

---

## Task 15: REVIEW per `superpowers:requesting-code-review`

**Files:**
- Create: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/REVIEW.md` (~280 LoC)
- Append: `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md` (Task 15 entry)

This task runs the end-of-phase code review per `superpowers:requesting-code-review`. Mirror phase-09..18.2 REVIEW.md structure. Verifies the 16 SPEC §15 acceptance items + the 6 phase-done gates per BOOTSTRAP_PROMPT §7.5 + the 8 ADRs landed at their respective Lands-in-Tasks per ADR-0044.

**Precondition:** Tasks 1-14 complete; all 6 phase-done gates verified GREEN at Task 14.
**Artifact:** `docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/REVIEW.md` (NEW).
**Acceptance:** REVIEW.md addresses all 16 SPEC §15 acceptance items + identifies the 6 phase-done gates as GREEN + lists the 8 ADRs landed (ADR-0167..ADR-0174) + confirms ADR-0044 escape-valve disposition (D12 hypothesis HELD or FALSIFIED with rationale) + lists any in-place ADR §Decision AMENDMENTS at Task 13 (parent §19.P4 / §19.P7 / §19.P8 closures); review-document-reviewer subagent or human reviewer approves.

- [ ] **Step 1: Invoke `superpowers:requesting-code-review`** — per the skill's discipline; provide PLAN.md path + SPEC.md path + STATE.md path. The skill dispatches the reviewer with the focused context.

- [ ] **Step 2: Address reviewer feedback** — fix issues in-place (per `superpowers:receiving-code-review`); iterate until approved.

- [ ] **Step 3: Author `REVIEW.md`** — `## Phase 19.1 review` + sections for each acceptance item (1-16); each section captures: "Acceptance criterion verbatim from SPEC §15", "Evidence of satisfaction (commit hash + file path + grep verification)", "Status: GREEN / RED-with-remediation". + `## Phase-done gates` (A/B/C/D/E/F per BOOTSTRAP §7.5) each marked GREEN with evidence. + `## ADR roster` listing the 8 ADRs landed + their Lands-in-Tasks + the ADR-0044 escape-valve disposition. + `## Forward-pointers carried into 19.2` (the 18 §8 deferred items as the 19.2 inheritance set).

- [ ] **Step 4: Append PROGRESS.md Task 15 entry** + record the reviewer approval signal.

- [ ] **Step 5: Commit**

```bash
git add docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/REVIEW.md \
        docs/envoy-go/phases/19.1-http-filter-ext-proc-headers/PROGRESS.md
git commit -m "phase 19.1 Task 15: REVIEW.md + reviewer approval

Phase 19.1 closing review per superpowers:requesting-code-review.
All 16 SPEC §15 acceptance items GREEN; all 6 phase-done gates GREEN;
8 ADRs landed (ADR-0167..ADR-0174) at their respective Lands-in-Tasks
per ADR-0044; ADR-0044 escape-valve held (D12 hypothesis HELD; or
FALSIFIED with ADR-0177 rationale). Forward-pointers carried into 19.2."
```

---

## Phase-done squash-merge + push to origin

After Task 15 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-19.1-http-filter-ext-proc-headers-impl
# Resolve commit message — body must include the 15-task summary + the
# 8-ADR roster + the closes-row-19.1 + parent-19-stays-in-progress note
git commit -m "$(cat <<'EOF'
Squash merge phase-19.1-http-filter-ext-proc-headers-impl

Closes ROADMAP row 19.1 (in-progress → done). Parent row 19 STAYS
in-progress per parent SPEC §8 rollup discipline (closes at 19.2's
phase-done AT THE SAME commit).

15 tasks landed. 8 ADRs anchored (ADR-0167..ADR-0174). 24th fuzzer
FuzzExtProcConfigParse clean. 23/23 differential fixtures green
(0000-0022). All 6 phase-done gates green.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..18.x convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 14):
# Edit docs/envoy-go/STATE.md replacing "<Task 14 commit SHA — TBD before squash; SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 19.1 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-19.1-http-filter-ext-proc-headers-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember
- Exact file paths always.
- Complete code shapes in the SPEC §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor).
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 12), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit), `@superpowers:requesting-code-review` (Task 15), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 14).
- DRY, YAGNI, TDD, frequent commits.

End of phase 19.1 PLAN.
