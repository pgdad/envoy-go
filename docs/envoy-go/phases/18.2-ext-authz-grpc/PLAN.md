# Phase 18.2 — HTTP filter `envoy.filters.http.ext_authz` (gRPC service mode + `internal/grpcclient/` primitive) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `envoy.filters.http.ext_authz` in **gRPC service mode** — completing the phase-18 ADR-0045 split begun at 18.1 — by activating the `*ExtAuthz_GrpcService` switch-arm in `compiledConfig` dispatch, shipping the NEW cross-phase `internal/grpcclient/` framework primitive (envoy-go's FIRST gRPC infrastructure of any kind), the gRPC-mode `AttributeContext` builder + `CheckResponse` → `checkDisposition` mapper, the NEW `test/helpers/extauthzgrpc/` in-process gRPC `Authorization/Check` server, the 23rd fuzzer `FuzzCheckResponseMapping`, and differential fixture `0021-http-ext-authz-grpc` — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on every observable axis except the documented divergence-windows. **The 18.2 phase-done commit closes BOTH row `18.2` AND parent row `18` per parent SPEC §8 rollup discipline (single grep-verifiable commit-message body).**

**Architecture:** REUSE the 18.1 `internal/filter/http/extauthz/` package surface unchanged — `compiledConfig` is field-final per ADR-0157 §Decision, `checkDisposition` + the disposition-application logic + the 6-counter `filterStats` + the deny-path `SendLocalReply` + the async-resume leg + the per-route 5th-canonical REUSE + the `with_request_body` ADR-0128 reuse + boot-registration ALL carry forward UNCHANGED. The 18.2-specific surface is: (1) NEW top-level `internal/grpcclient/` package (`Dialer` + `AuthClient`) coupling to `internal/cluster.Manager` for `EnvoyGrpc.cluster_name` resolution via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure)` (TLS terminates at the cluster-manager layer per the §11.P13 in-session SPEC scrape; ADR-0158); (2) `buildGRPCCheckFn` in `check.go` replacing the 18.1 PARSE-REJECT (ADR-0157 §Decision AMENDMENT — activates the `grpc_service` arm; `GoogleGrpc` arm PARSE-REJECTs envoy-go-strict; `initial_metadata`/`retry_policy` SILENT-IGNORED); (3) `buildAttributeContext` in `attributes.go` (ADR-0160 gRPC-mode portion — `source`/`destination` Peers, `request.http` per parent §5.P4 + §11.P4 refinements, `request.time` as `Timestamp`, `tls_session.sni` gated by `include_tls_session`, `source.certificate` gated by `include_peer_certificate`, `destination.principal` populated AUTOMATICALLY from listener TLS cert per §11.P4 in-session, `context_extensions` listener+per-route merge, `encode_raw_headers` `headers`-vs-`header_map` discipline); (4) `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` in `check.go` (ADR-0161 gRPC-mode portion — `OkHttpResponse` allow-path set/append per `append_action`; `DeniedHttpResponse` deny-path verbatim header pass-through UNLIKE HTTP-mode's matcher-filtered headers); (5) `*authRequest` extended at 18.2 to carry the per-stream state `buildAttributeContext` needs (`remoteAddr`/`localAddr`/`tlsServerName`/`peerCertDER`/`listenerPrincipal`/`protocol`/`streamStartTime`/`requestID`/`perRouteContextExtensions`/`downstreamPrincipal` fields) — captured at `DecodeHeaders` time via the NEW callback-surface extension (planner-time decision D3 + ADR-0044 escape-valve firing → ADR-0165 anchors the callback group as a cross-phase-reusable framework primitive for ext_proc + global_ratelimit; SPEC §13.5 is AMENDED in-place at the Task-4 commit). The gRPC-specific config (`include_*` gates, `encode_raw_headers`, `pack_as_bytes`) is captured DIRECTLY in the closure's lexical scope inside `buildGRPCCheckFn`, NOT promoted to `compiledConfig` struct fields per §6.5 step 5. Differential fixture `0021-http-ext-authz-grpc` with 8 scenarios across a three-listener topology (l_test_a/b/c for `failure_mode_allow` scoping). NEW `test/helpers/extauthzgrpc/` — the FIRST in-process gRPC server in envoy-go's test tree (plaintext h2c on ephemeral port; `:path`-keyed scriptable `CheckResponse` per planner-time decision D1). File layout: extend existing `check.go` (+250–400 LoC) + `attributes.go` (+200–350 LoC); NO new `grpc_*.go` files per SPEC §6.8. `internal/grpcclient/` is the only NEW directory in 18.2.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/ext_authz/v3` + `envoy/service/auth/v3` + `envoy/config/core/v3` + `envoy/type/v3`); `google.golang.org/grpc` v1.70.0 (already an indirect module dep at master tip `be18857` — PROMOTED to direct by 18.2 Task 2); `google.golang.org/grpc/credentials/insecure` for the gRPC transport-credentials no-op handshaker (TLS terminates at the cluster-manager layer per §11.P13); `google.golang.org/grpc/resolver` (the `passthrough:///` resolver scheme is built-in to gRPC and requires no extra import — planner-time decision D4); `google.golang.org/protobuf/types/known/timestamppb` for `request.time` construction (`timestamppb.Now()`); `protojson`/`anypb` for proto decoding; `context.Context` for per-Check cancellable outbound calls (threaded into `*grpc.ClientConn.Invoke`); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext h2c auth-cluster fixture (no TLS-to-auth-cluster fixture scenario — the §11.P13 in-session SPEC scrape RATIFIED the TLS-to-auth-cluster path against reference Envoy; behavioral verification of envoy-go's TLS-aware AttributeContext fields lives in unit tests against mocked `*authRequest` state per SPEC §7.2 known-testing-gap note).

---

## Scope check — why phase 18.2 ships as one row (it already is the split half)

Phase 18 was SPLIT into `18.1-ext-authz-http` + `18.2-ext-authz-grpc` at the phase-18 SPEC commit (`308e9b6`) per ADR-0045 / ADR-0164. 18.1 closed `done` at 2026-05-15 (parent row 18 stays `in-progress` per parent SPEC §8 rollup; closes at 18.2's IMPL phase-done AT THE SAME COMMIT). This PLAN is for the 18.2 sub-phase ONLY (gRPC service mode + the NEW `internal/grpcclient/` primitive); no further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward).

Net change estimate for 18.2 (mirroring the phase-09..18.1 PLAN component-table convention):

- `internal/grpcclient/doc.go` ~30
- `internal/grpcclient/grpcclient.go` ~150–250 (`Dialer` + `AuthClient` + `New` + `DialContext` + `NewAuthClient` + `Check` + `Close` + `passthrough:///` resolver wiring + `WithContextDialer(cluster.Dial)` + `WithTransportCredentials(insecure.NewCredentials())`)
- `internal/grpcclient/grpcclient_test.go` ~220–320 (Groups 1+2+3 per SPEC §14.1 — `Dialer.DialContext` happy/PARSE-REJECT/UseH2-false; `AuthClient.Check` happy + timeout-propagation + context-cancel; `AuthClient.Close` idempotency)
- `internal/filter/http/extauthz/check.go` ~+250–400 (`buildGRPCCheckFn` + `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC`)
- `internal/filter/http/extauthz/attributes.go` ~+200–350 (`buildAttributeContext` + `lowercaseHeaderMap` helper + `injectHCMRequiredHeaders` helper + `populateTLSSession`/`populatePeerCertificate` gate helpers)
- `internal/filter/http/extauthz/extauthz.go` ~+50–80 (`grpc_service` switch-arm activation in `buildCompiledConfig`; extended `*authRequest` field set; `dispatchOutboundCheck` seeds extended fields from new callbacks)
- `internal/filter/http/extauthz/extauthz_test.go` ~+700–1100 (Groups 10/11/12/13/14 per SPEC §14.1)
- `internal/filter/http/extauthz/fuzz_test.go` ~+85 (23rd fuzzer `FuzzCheckResponseMapping`; corpus extension of existing `FuzzExtAuthzConfigParse` with `grpc_service` variants — same file)
- `internal/filter/http/callbacks.go` ~+60–100 (6 new `DecoderFilterCallbacks` methods: `DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`, `ListenerPrincipal` — D3 + ADR-0165)
- `internal/filter/http/chain.go` ~+100–150 (6 new chain seeding primitives `SetDownstreamRemoteAddr` / `SetDownstreamLocalAddr` / `SetDownstreamTLSServerName` / `SetDownstreamTLSPeerCertDER` / `SetDownstreamProtocol` / `SetListenerPrincipal` + matching `*decoderCB` readers + chain fields)
- `internal/filter/hcm/connection.go` ~+30 (H1 dispatch site — 6 new chain seeding calls alongside the existing `SetTLSPrincipals(downstreamTLSPrincipals(downstream))`)
- `internal/filter/hcm/h2dispatch.go` ~+30 (H2 dispatch site — same 6 new chain seeding calls alongside `chainDispatchAction.tlsPrincipals` plumbing)
- `internal/filter/http/chain_test.go` ~+100–150 (6 new chain seed/read round-trip tests)
- `test/helpers/extauthzgrpc/doc.go` ~25 + `test/helpers/extauthzgrpc/extauthzgrpc.go` ~150–220 + `test/helpers/extauthzgrpc/extauthzgrpc_test.go` ~100–140 (NEW test-helper — FIRST in-process gRPC server in envoy-go's test tree)
- `test/differential/fixture/fixture.go` ~+15 (`HTTPExtAuthzGRPC BackendKind = 18`)
- `test/differential/runner_test.go` ~+12 (blank import + switch-case)
- `test/fixtures/0021-http-ext-authz-grpc/` (NEW DIRECTORY) — `envoy.yaml` ~200 + `envoy-go.yaml` ~200 + `expectations.yaml` ~85 + `README.md` ~130 + `inputs/driver.go` ~350 = ~965
- `docs/envoy-go/DECISIONS.md` — 4 ADRs landed at 18.2 IMPL: ADR-0158 (§Decision + §Consequences; §Context already at parent SPEC commit) + ADR-0157 §Decision AMENDMENT in-place edit + ADR-0160 gRPC-mode §Decision + §Consequences (extends existing ADR-0160) + ADR-0161 gRPC-mode §Decision + §Consequences (extends existing ADR-0161); ~+250 LoC. Optional 5th: ADR-0165 (the callback-surface extension framework primitive per D3 + ADR-0044 escape-valve) — ~+90 LoC; lands at Task 4.
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+280 (§13 8-edit bundle per SPEC §13)
- `docs/envoy-go/ROADMAP.md` rows 18.2 + 18 BOTH flip `in-progress → done` AT THE SAME COMMIT per parent SPEC §8 rollup; ~+2 net
- `docs/envoy-go/STATE.md` rewrite-in-place
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (NEW) ~600 + `REVIEW.md` (NEW) ~240

**Production code: ~750–1300 LoC** (`internal/grpcclient/` ~180–280 + extauthz extensions ~500–830 + callback-surface extension ~190–280 + HCM seeding ~60) **+ ~135–185 LoC test-helper = ~885–1485 LoC production** + ~1050–1700 LoC tests + ~965 LoC fixture + ~620 LoC docs (incl. ADR-0165 if it fires) ≈ **~3520–4770 LoC total**. Task count below is **14** — comfortably under the ADR-0045 25-task split-gate. The production-LoC high-end (~1485) brushes the ~1500-LoC soft threshold but the task-count gate is load-bearing, not LoC (per the phase-13..18.1 LoC-borderline precedent + phase-18.2 IS ALREADY a sub-phase row), so **18.2 ships as the single row it is** — no further split.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/grpcclient/doc.go` | NEW | Package doc enumerating: (a) the `Dialer` API (cluster-name → `*grpc.ClientConn` via `internal/cluster.Manager` coupling); (b) the `AuthClient` typed wrapper (`envoy.service.auth.v3.AuthorizationClient` stub from `go-control-plane v1.32.4` — no codegen); (c) the connection lifecycle (one `*grpc.ClientConn` per (cluster_name, compiledConfig) pair created at config-load time; leaks-on-exit MVP per planner-time decision D2); (d) cross-phase reuse intent (ext_proc + global_ratelimit per ADR-0158 §Consequences); (e) the TLS-at-cluster-manager-layer integration (the `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` pattern — the §11.P13 in-session SPEC scrape RATIFICATION); (f) ADR anchors (ADR-0158). Mirrors `internal/jwks/doc.go` shape (the phase-17 outbound-HTTP primitive precedent). Per SPEC §3.1 + §6.5 step 3. ~30 LoC. |
| `internal/grpcclient/grpcclient.go` | NEW | Main file. **Public surface** (per SPEC §3.1): `Dialer` struct + `New(mgr *cluster.Manager) *Dialer` + `DialContext(ctx context.Context, clusterName string) (*grpc.ClientConn, error)` (PARSE-REJECT via error return when cluster does not exist OR `UseH2()==false`; uses `grpc.NewClient("passthrough:///"+clusterName, grpc.WithContextDialer(...), grpc.WithTransportCredentials(insecure.NewCredentials()))` — the `passthrough:///` scheme is gRPC's built-in single-endpoint resolver and skips DNS, delegating endpoint selection to the cluster manager's `(*Cluster).Dial(ctx)` per planner-time decision D4); `AuthClient` struct (`conn *grpc.ClientConn`, `stub authv3.AuthorizationClient`, `target string`) + `NewAuthClient(d *Dialer, clusterName string, timeout time.Duration) (*AuthClient, error)` + `(*AuthClient) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error)` (per-Check `context.WithTimeout(ctx, timeout)` is applied INSIDE `Check` per SPEC §3.1 + planner-time decision D7 — transport-level errors (`Unavailable` / `DeadlineExceeded` / `Canceled`) propagate verbatim to caller for the caller to map to `dispError`; the SPEC §6.7 `mapGRPCResponse` operates on the `*authv3.CheckResponse`, never on a transport error — transport errors return via the `error` return of `Check`) + `(*AuthClient) Close() error` (idempotent close via internal sync.Once guard). The `passthrough:///` URL form: `grpc.NewClient` parses the URL, the resolver returns the literal endpoint without DNS, and our `WithContextDialer` callback receives the bare cluster name as the target string (which we ignore — we re-look-up via the cluster manager). ~150–250 LoC. Per SPEC §3.1 + §6.5 step 3. ADR-0158 §Decision + §Consequences land at Task 3. |
| `internal/grpcclient/grpcclient_test.go` | NEW | Unit tests per SPEC §14.1 — **Group 1 (Dialer):** `New` returns non-nil; `DialContext` happy path (plaintext h2c cluster via a fake `cluster.Manager` + a real `*grpc.Server`); `DialContext` PARSE-REJECT on unknown cluster name; `DialContext` PARSE-REJECT on `UseH2()==false`; concurrent `DialContext` calls return distinct ClientConns (one-per-call discipline — caller owns Close). **Group 2 (AuthClient):** `NewAuthClient` happy path + propagates `DialContext` errors; `Check` happy path returns the scripted response; `Check` honors `context.WithTimeout` (per-Check timeout per planner-time decision D7); `Check` honors caller `ctx.Done()` (the OnDestroy-cancel propagation per SPEC §14.2); `Check` returns transport-error verbatim (does NOT map to `dispError` — that's the filter's responsibility per SPEC §6.7 + planner-time decision D7). **Group 3 (Close):** `Close()` idempotent (second call returns nil; underlying ClientConn closed only once); concurrent `Close()` calls race-clean under `-race`. ~220–320 LoC. |
| `internal/filter/http/extauthz/check.go` | MODIFIED | Extends the existing 18.1 file with the gRPC-mode `checkFn` + response mapping per SPEC §6.5 + §6.7. **`buildGRPCCheckFn(gs *core.GrpcService, ctx envoyhttp.FactoryCtx, validateMutations, includePeerCertificate, includeTlsSession, encodeRawHeaders, packAsBytes bool) (checkFn, error)`** — per SPEC §6.5: (1) `GoogleGrpc` arm PARSE-REJECT envoy-go-strict — `errors.New("ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)")`; (2) `EnvoyGrpc.cluster_name` PGV-mirror (`min_len: 1` — PARSE-REJECT empty); (3) cluster-manager lookup via `ctx.ClusterManager.Get(cluster_name)` — PARSE-REJECT on unknown cluster; PARSE-REJECT on `!cluster.UseH2()` (`http2_protocol_options{}` MUST be set on the auth cluster for gRPC framing per the §11.P13 in-session scrape); (4) construct `*grpcclient.AuthClient` via `grpcclient.New(ctx.ClusterManager)` + `grpcclient.NewAuthClient(dialer, cluster_name, durationpbToGo(gs.Timeout))`; (5) `initial_metadata` + `retry_policy` SILENT-IGNORED per SPEC §2.6 + §8 items 2+3; (6) return the closure capturing `*AuthClient` + the four boolean gates + `validateMutations`. The closure body: `attrCtx := buildAttributeContext(req, encodeRawHeaders, packAsBytes, includePeerCertificate, includeTlsSession); checkReq := &authv3.CheckRequest{Attributes: attrCtx}; resp, err := ac.Check(ctx, checkReq); if err != nil { return checkDisposition{class: dispError}, err }; return mapGRPCResponse(resp, validateMutations), nil`. **`mapGRPCResponse(resp *authv3.CheckResponse, validateMutations bool) checkDisposition`** — per SPEC §6.7: dispatch on `resp.HttpResponse` oneof + `resp.GetStatus().GetCode()` per the SPEC §6.7 truth table (empty oneof + status==0 → allow; empty oneof + status!=0 → deny with default 403; `*CheckResponse_OkResponse` + status==0 → allow via `buildAllowDispositionGRPC`; `*CheckResponse_OkResponse` + status!=0 → dispError (structurally inconsistent — envoy-go-strict catches auth-server bugs per SPEC §6.7 commentary); `*CheckResponse_DeniedResponse` + status!=0 → deny via `buildDenyDispositionGRPC`; `*CheckResponse_DeniedResponse` + status==0 → dispError (BEHAVIOR_CONTRACT-documented divergence-window per SPEC §6.7 commentary + §13.4)). **`buildAllowDispositionGRPC(okResp *OkHttpResponse, validateMutations bool) checkDisposition`** — extract `OkHttpResponse.headers` into `upstreamSet` / `upstreamApp` per the 4-arm `append_action` dispatch table (planner-time decision D5: `OVERWRITE_IF_EXISTS_OR_ADD` + `ADD_IF_ABSENT` → `upstreamSet`; `APPEND_IF_EXISTS_OR_ADD` → `upstreamApp`; `OVERWRITE_IF_EXISTS` → `upstreamSet` BUT mark `addIfAbsent: false` — a SET-IF-PRESENT semantic that the `applyUpstreamMutations` helper honors via a new `headerKV.setIfAbsent` discriminator field OR by interpretation in the apply step; the IMPL settles the exact representation); extract `OkHttpResponse.headers_to_remove` into a NEW `upstreamDel []string` field on `checkDisposition` (the 18.2 IMPL extends `checkDisposition` with `upstreamDel`); `response_headers_to_add` DEFERRED per SPEC §8 item 5. **`buildDenyDispositionGRPC(deniedResp *DeniedHttpResponse, outerStatus int32) checkDisposition`** — extract `DeniedHttpResponse.status.code` → `denyStatus` (default 403 when zero per SPEC §6.7); extract `DeniedHttpResponse.body` verbatim → `denyBody`; extract `DeniedHttpResponse.headers` VERBATIM (NO `allowed_client_headers` filter — UNLIKE HTTP-mode; per SPEC §4 + parent §5.P11). `validateMutationHeaders` gating (the same 18.1 routine in `attributes.go`) applies identically to both modes — a violation drives `dispInvalid`. ~+250–400 LoC. ADR-0161 gRPC-mode portion lands at Task 6. |
| `internal/filter/http/extauthz/attributes.go` | MODIFIED | Extends the existing 18.1 file with the gRPC-mode `AttributeContext` builder per SPEC §6.6. **`buildAttributeContext(req *authRequest, encodeRawHeaders, packAsBytes, includePeerCert, includeTlsSession bool) *authv3.AttributeContext`** — pure function of `*authRequest` + the four boolean gates (NO `DecoderFilterCallbacks` parameter — all per-stream state read from the extended `*authRequest`). Steps per SPEC §6.6: (1) build `source = &Peer{Address: addressFromNetAddr(req.remoteAddr), Principal: firstOrEmpty(req.downstreamPrincipal)}` per parent §5.P3 + ADR-0144; (2) build `destination = &Peer{Address: addressFromNetAddr(req.localAddr), Principal: req.listenerPrincipal}` (listener-principal populated AUTOMATICALLY per §11.P4 in-session SPEC scrape — NOT gated by `include_peer_certificate`); (3) build `request.http = &AttributeContext_HttpRequest{Id: req.requestID, Method: req.method, Headers: lowercaseHeaderMap(req.headers), Path: req.path, Host: req.headers.Get(":authority"), Scheme: req.headers.Get(":scheme"), Size: int64(len(req.body)), Protocol: req.protocol, Body: bodyStringIfNotBytes(req.body, packAsBytes), RawBody: bodyBytesIfBytes(req.body, packAsBytes)}` — pseudo-headers `:authority`/`:method`/`:path`/`:scheme` INCLUDED in the lowercased headers map per §11.P4; HCM-injected `x-forwarded-proto`/`x-request-id`/`x-envoy-auth-partial-body` are already in `req.headers` by the time DecodeHeaders runs per §11.P4 finding; (4) build `request.time = timestamppb.New(req.streamStartTime)` (or `timestamppb.Now()` if streamStartTime is zero — the IMPL settles); (5) IF `includeTlsSession` AND `req.tlsServerName != ""`: populate `tls_session = &AttributeContext_TLSSession{Sni: req.tlsServerName}` (ONLY `sni` populated per §11.P4 in-session evidence; other TLSSession fields stay empty); (6) IF `includePeerCert` AND `len(req.peerCertDER) > 0`: populate `source.Certificate = req.peerCertDER` (DER-encoded leaf cert); (7) IF `encodeRawHeaders` (planner-time decision D6 — DEFERRED for MVP; the flag PARSES but the `header_map` field is NOT populated — the legacy `headers` map suffices for fixture 0021's byte-equivalence assertion since reference Envoy populates `headers` by default; SPEC §8 item 8 records the conditional deferral): no-op for MVP; the `header_map` field stays empty (legacy `headers` always populated); (8) set `context_extensions = req.perRouteContextExtensions` (the merged map from the per-route resolved `CheckSettings.context_extensions` — empty for MVP-no-per-route; per SPEC §5); (9) set `metadata_context = &Metadata{}` + `route_metadata_context = &Metadata{}` (empty-proto-message; populated as empty per §11.P4 in-session evidence — deferred dynamic-metadata family per SPEC §8 item 1); (10) return. **Helpers added:** `addressFromNetAddr(net.Addr) *core.Address` (wraps in `&core.Address{Address: &core.Address_SocketAddress{SocketAddress: &core.SocketAddress{Address: ip, PortSpecifier: &core.SocketAddress_PortValue{PortValue: port}}}}`); `lowercaseHeaderMap(http.Header) map[string]string` (single-value-per-key — multi-value headers join with `,` per reference Envoy; the §11.P4 evidence shows single-value rendering); `firstOrEmpty([]string) string`; `bodyStringIfNotBytes` / `bodyBytesIfBytes`. ~+200–350 LoC. ADR-0160 gRPC-mode portion lands at Task 5. |
| `internal/filter/http/extauthz/extauthz.go` | MODIFIED | Extends the existing 18.1 file in three places: **(1) `buildCompiledConfig` `services` oneof dispatch** — replace the 18.1 `*ExtAuthz_GrpcService` PARSE-REJECT with a call to `buildGRPCCheckFn(s.GrpcService, ctx, cc.validateMutations, raw.GetIncludePeerCertificate(), raw.GetIncludeTlsSession(), raw.GetEncodeRawHeaders(), packAsBytesFromWRB(cc.withRequestBody))` (per SPEC §6.4 — ADR-0157 §Decision AMENDMENT). The `compiledConfig` struct shape is UNCHANGED (field-final at 18.1 per ADR-0157 §Decision). **(2) `*authRequest` type extension** — add fields `remoteAddr net.Addr`, `localAddr net.Addr`, `tlsServerName string`, `peerCertDER []byte`, `listenerPrincipal string`, `protocol string` (e.g. `"HTTP/1.1"` / `"HTTP/2"`), `requestID string`, `streamStartTime time.Time`, `perRouteContextExtensions map[string]string`, `downstreamPrincipal []string` (the ADR-0144 reuse). The 18.1 fields (`method`, `path`, `headers`, `body`) carry forward unchanged — the closure signature `(ctx, *authRequest)` stays mode-agnostic per ADR-0157 §Decision. **(3) `dispatchOutboundCheck` extension** — seed the new `*authRequest` fields from the new callbacks: `req.remoteAddr = f.dcb.DownstreamRemoteAddr()`; `req.localAddr = f.dcb.DownstreamLocalAddr()`; `req.tlsServerName = f.dcb.DownstreamTLSServerName()`; `req.peerCertDER = f.dcb.DownstreamTLSPeerCertDER()`; `req.listenerPrincipal = f.dcb.ListenerPrincipal()`; `req.downstreamPrincipal = f.dcb.DownstreamPrincipal()` (ADR-0144 reuse); `req.protocol = f.dcb.DownstreamProtocol()`; `req.requestID = headers.Get("x-request-id")`; `req.streamStartTime = time.Now()` (or threaded from `f.streamStartTime` if the IMPL captures at DecodeHeaders entry); `req.perRouteContextExtensions = perRouteContextExtensionsFor(f.perRoute)`. **NOTE:** all new callback methods land at Task 4 (D3 + ADR-0165) BEFORE this seeding wires up. The 18.1 HTTP-mode closure ignores the new fields (the `*authRequest` extension is gRPC-mode-only-consumed); the HTTP-mode closure's behavior is unchanged. ~+50–80 LoC. ADR-0157 §Decision AMENDMENT lands at Task 3. |
| `internal/filter/http/extauthz/extauthz_test.go` | MODIFIED | Unit tests per SPEC §14.1 — single file extension (no split per planner-time decision D8). **Group 10 (NEW) — `buildGRPCCheckFn` parse-time validation:** unknown cluster → PARSE-REJECT; `UseH2() == false` → PARSE-REJECT; `GoogleGrpc` arm → PARSE-REJECT envoy-go-strict; `EnvoyGrpc.cluster_name` empty → PARSE-REJECT; happy path (known h2 cluster) → returns non-nil `checkFn`. **Group 11 (NEW) — `mapGRPCResponse` mapping:** OK+OkResponse{} → allow; OK+nil-oneof → allow (defensive empty CheckResponse); OK+DeniedResponse → dispError (structurally inconsistent — auth-server bug surface per SPEC §6.7); non-zero status + nil-oneof → deny default 403; non-zero status + DeniedResponse → deny with body+headers verbatim; non-zero status + OkResponse → dispError; `OkHttpResponse.headers` with all 4 `append_action` enum values → correct upstreamSet/upstreamApp/upstreamDel population per D5; `validate_mutations:true` on a `:`-prefixed pseudo-header → dispInvalid + `invalid` counter; `OkHttpResponse.response_headers_to_add` (DEFERRED) → silent-ignored (no crash). **Group 12 (NEW) — `buildAttributeContext`:** populated set per §11.P4 evidence — pseudo-headers in the `headers` map lowercased; `request.time` non-zero; `source/destination.address.socket_address` from `req.remoteAddr/localAddr`; `source.principal` from first of `req.downstreamPrincipal`; `destination.principal` from `req.listenerPrincipal` (populated AUTOMATICALLY — NOT gated); `tls_session.sni` populated ONLY when `includeTlsSession:true` AND `req.tlsServerName != ""`; `source.certificate` populated ONLY when `includePeerCert:true` AND `len(req.peerCertDER) > 0`; `metadata_context` + `route_metadata_context` populated as empty messages (NOT nil); `context_extensions` populated from `req.perRouteContextExtensions`; `pack_as_bytes:false` → `request.http.body` populated as string; `pack_as_bytes:true` → `request.http.raw_body` populated as bytes; `encodeRawHeaders` (D6 — DEFERRED) → `header_map` stays empty (legacy `headers` populated). **Group 13 (NEW) — extended callback surface (D3 + ADR-0165):** the 4 new `DecoderFilterCallbacks` methods return seeded chain values (TLS connection: `DownstreamTLSServerName` returns the seeded SNI; plaintext: returns ""); chain-seed-and-read round-trip; nil-tolerance for unseeded fields (e.g. `DownstreamTLSPeerCertDER` returns nil on non-TLS connections). **Group 14 (NEW) — gRPC-mode integration:** end-to-end through `dispatchOutboundCheck` against a scripted gRPC server (`test/helpers/extauthzgrpc/`) — allow / deny / error / invalid paths; per-route `context_extensions` flows into `AttributeContext.context_extensions`; the 6 mode-agnostic counter increments match 18.1's HTTP-mode behavior; `OnDestroy` cancels the in-flight `Check` call (cancellation propagates through `*grpc.ClientConn.Invoke` and returns `context.Canceled` → `dispError` + `failure_mode_allow` posture). ~+700–1100 LoC. |
| `internal/filter/http/extauthz/fuzz_test.go` | MODIFIED | NEW 23rd fuzzer `FuzzCheckResponseMapping` per SPEC §7.3: fuzz arbitrary bytes as a `*authv3.CheckResponse` proto-bytes input → `proto.Unmarshal` → `mapGRPCResponse` → `checkDisposition`; assertions: disp.class ∈ {dispAllow, dispDeny, dispError, dispInvalid}; on deny — denyStatus ∈ [100,599]; denyBody is a copy (not aliased); denyHeaders passes `validateMutationHeaders` when `validate_mutations:true`. Corpus seeds (6–10 variants): valid OK+OkResponse{}; OK+OkResponse with mutations (each `append_action` arm); OK+DeniedResponse (structurally inconsistent — should map to dispError); non-OK+DeniedResponse with various status codes (zero, 401, 403, 500, 999); non-OK+OkResponse (structurally inconsistent — should map to dispError); empty CheckResponse{}; oversized header values; invalid status codes (1000+); pseudo-header in mutation headers (→ validate_mutations rejection). 30s/seed under ADR-0018 budget. ALSO: extend the existing `FuzzExtAuthzConfigParse` corpus with `grpc_service` config variants (`EnvoyGrpc.cluster_name` valid/empty/unknown; `GoogleGrpc` arm; `initial_metadata` populated; `retry_policy` populated; `transport_api_version` non-V3) — same fuzzer, just corpus growth. ~+85 LoC. |
| `cmd/envoy-go/main.go` | UNCHANGED | extauthz is already boot-registered at 18.1 (between `envoygotest` and `fault` per ADR-0100 §2.2). 18.2 does NOT touch boot wiring per SPEC §2.9. |
| `internal/filter/http/callbacks.go` | MODIFIED | Add 6 new methods to `DecoderFilterCallbacks` per planner-time decision D3 + ADR-0165 (the ADR-0044 escape-valve firing): `DownstreamRemoteAddr() net.Addr` (returns nil on synthetic streams); `DownstreamLocalAddr() net.Addr`; `DownstreamTLSServerName() string` (empty for plaintext / SNI-absent); `DownstreamTLSPeerCertDER() []byte` (nil for plaintext / no-client-cert); `DownstreamProtocol() string` (`"HTTP/1.1"` for H1; `"HTTP/2"` for H2 — read from chain field seeded at dispatch); `ListenerPrincipal() string` (empty for plaintext listener; derived from listener TLS leaf cert SAN/CN — populated AUTOMATICALLY per §11.P4 in-session SPEC scrape). Doc-comments cite ADR-0165 + the cross-phase reuse intent (ext_proc + global_ratelimit + future `ext_authz` extensions). **D3-DEVIATION-FROM-SPEC §13.5 NOTE:** SPEC §13.5 stated "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; the planner-time settle of D3 surfaces that the SPEC's hard constraint is in direct conflict with SPEC §15 acceptance item 4 (populated `tls_session.sni` + `source.certificate` + socket addresses + `destination.principal` per §11.P4 RATIFICATION). The PLAN settles by AMENDING SPEC §13.5 + §6.5 step 5 + §6.6 step 1-2 at Task 4 (all three places carry the "NO new callback method" claim — each must be flipped for SPEC internal consistency). The callback-surface extension is unavoidable; the alternative (UNPOPULATED tls_session.sni etc.) is a behaviorally significant divergence vs reference Envoy and SPEC §15 item 4. ADR-0165 anchors the callback-group as a cross-phase-reusable framework primitive. ~+60–100 LoC. Task 4. |
| `internal/filter/http/chain.go` | MODIFIED | Add 6 chain seeding primitives + 6 chain fields + 6 `*decoderCB` reader methods, mirroring the `tlsPrincipals` / `SetTLSPrincipals` / `(*decoderCB) DownstreamPrincipal` pattern at chain.go:107 + 551 + 483. Fields: `downstreamRemoteAddr net.Addr`, `downstreamLocalAddr net.Addr`, `downstreamTLSServerName string`, `downstreamTLSPeerCertDER []byte`, `downstreamProtocol string`, `listenerPrincipal string`. Setters: `SetDownstreamRemoteAddr(net.Addr)`, `SetDownstreamLocalAddr(net.Addr)`, `SetDownstreamTLSServerName(string)`, `SetDownstreamTLSPeerCertDER([]byte)`, `SetDownstreamProtocol(string)`, `SetListenerPrincipal(string)` — all single-set-then-read per the `tlsPrincipals` discipline (set ONCE at chain build time by HCM dispatch BEFORE `RunDecodeHeaders`; read concurrently by per-stream callbacks). Readers on `*decoderCB`: 6 new methods returning the chain field. ~+100–150 LoC. Task 4. |
| `internal/filter/http/chain_test.go` | MODIFIED | 6 new chain seed/read round-trip tests mirroring `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed` (chain_test.go:1507): one test per new field — seed via `Set...`, read via the new callback method, assert returned value matches. Plus nil/empty fall-throughs (unset chain → reader returns zero value). ~+100–150 LoC. Task 4. |
| `internal/filter/hcm/connection.go` | MODIFIED | Extend the H1 dispatch site at `dispatchRequest` (after the existing `chain.SetTLSPrincipals(downstreamTLSPrincipals(downstream))` at line ~311) with 6 new seeding calls: `chain.SetDownstreamRemoteAddr(downstream.RemoteAddr())`; `chain.SetDownstreamLocalAddr(downstream.LocalAddr())`; if `downstream` is `*tls.Conn`: `state := tlsConn.ConnectionState(); chain.SetDownstreamTLSServerName(state.ServerName); if len(state.PeerCertificates) > 0: chain.SetDownstreamTLSPeerCertDER(state.PeerCertificates[0].Raw)` (mirrors the existing `downstreamTLSPrincipals` pattern); `chain.SetDownstreamProtocol("HTTP/1.1")`; `chain.SetListenerPrincipal(listenerPrincipalFor(downstream))` (a new helper extracting the listener's leaf-cert SAN[0]/CN — looked up via the listener's `*stdtls.Config` per the §11.P4 in-session scrape — listener-principal is the LISTENER cert side, not the client cert). **PLAN-time-flagged sub-decision** (per reviewer note + Task 4 Step 0 pre-spike): if the listener `*stdtls.Config` is NOT reachable from `connection.go:dispatchRequest` via the existing parameters at master tip, Task 4 ~+30 LoC budget for `connection.go` may blow out; mitigation requires lifting a new parameter through the dispatch chain. The Task 4 implementer runs a 5-minute pre-spike (`grep` for how the listener TLS config currently flows to dispatchRequest) BEFORE writing tests to tighten the LoC budget realism. ~+30–80 LoC depending on the pre-spike outcome. Task 4. |
| `internal/filter/hcm/h2dispatch.go` | MODIFIED | Extend the H2 dispatch site at `chainDispatchAction.WriteH2` (or the chain-build callsite — same pattern as the existing H1 `connection.go`). Same 6 seeding calls as H1, with `chain.SetDownstreamProtocol("HTTP/2")` (vs `"HTTP/1.1"` on H1). The H2 path threads `tlsPrincipals` into `chainDispatchAction` as a struct field — the IMPL adds analogous fields (`downstreamRemoteAddr` etc.) to `chainDispatchAction` so the goroutine-safe handoff to the chain matches the existing pattern. ~+30 LoC. Task 4. |
| `test/helpers/extauthzgrpc/doc.go` | NEW | Package doc — `// Package extauthzgrpc implements a minimal in-process scriptable Authorization/Check gRPC server for differential fixtures whose driver needs to wire an ext_authz grpc_service endpoint into both envoy.yaml and envoy-go.yaml. Used by phase 18.2 fixture 0021-http-ext-authz-grpc. THE FIRST in-process gRPC server in envoy-go's test tree. Lifecycle: spawn-per-fixture; the runner allocates a free TCP port, starts the server via New(t), wires the EnvoyGrpc.cluster_name to a cluster pointing at that port in both yaml configs, runs the scenarios, then stops via Stop(). Plaintext h2c (no TLS) per SPEC §7.2; per-:path scriptable CheckResponse per planner-time decision D1.` ~25 LoC. |
| `test/helpers/extauthzgrpc/extauthzgrpc.go` | NEW | In-process scriptable gRPC `Authorization/Check` server per SPEC §7.4. **Public API:** `Server` type carrying `addr string` + `grpcSrv *grpc.Server` + `scripts map[string]*authv3.CheckResponse` + `mu sync.RWMutex`. `New(t testing.TB) *Server` — listens on `127.0.0.1:0` (ephemeral); registers `authv3.RegisterAuthorizationServer(grpcSrv, s)`; spawns `grpcSrv.Serve(lis)` in a goroutine; calls `t.Cleanup(s.Stop)`. `(s *Server) Addr() string` returns `lis.Addr().String()`. `(s *Server) Script(path string, resp *authv3.CheckResponse)` — registers a scripted `CheckResponse` for the discriminator key (the `:path` value from `req.Attributes.Request.Http.Path` per planner-time decision D1). `(s *Server) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error)` — looks up the scripted response by `:path`, returns it; returns `status.Errorf(codes.Unavailable, "no script registered for path %q", path)` when no script matches (which the filter maps to `dispError`). `(s *Server) Stop()` — `grpcSrv.GracefulStop()`. Plaintext h2c — `grpc.NewServer()` with no `Creds()` option. Per SPEC §7.4. ~150–220 LoC. |
| `test/helpers/extauthzgrpc/extauthzgrpc_test.go` | NEW | Unit tests: `TestNew_StartsServerOnEphemeralPort` (Addr() returns non-empty host:port); `TestServer_Script_ReturnsScripted` (registered script returns at Check; unregistered path returns `Unavailable`); `TestServer_Stop_Closes`; `TestServer_ConcurrentClient_NoRace` (under `-race`). ~100–140 LoC. |
| `test/differential/fixture/fixture.go` | MODIFIED | NEW `BackendKind` enum value `HTTPExtAuthzGRPC BackendKind = 18` after `HTTPExtAuthzHTTP BackendKind = 17`. Doc-comment: "HTTPExtAuthzGRPC reuses the existing echobackend helper at `test/helpers/echobackend/cmd/echobackend/main.go` for the upstream route + the NEW extauthzgrpc helper at `test/helpers/extauthzgrpc/` for the in-process gRPC auth server. 2-cluster topology (three HCM listeners l_test_a/b/c plaintext with `ext_authz → router` filter chain + cluster `c_backend` → echobackend subprocess + cluster `c_authz_grpc` → extauthzgrpc subprocess with `http2_protocol_options: {}`). No TLS — phase 18.2 fixture is HTTP/1.1 plaintext downstream + plaintext h2c auth cluster per SPEC §7.2 + §11.P13 in-session scrape RATIFICATION of TLS-to-auth-cluster." ~+15 LoC. Task 9. |
| `test/differential/runner_test.go` | MODIFIED | NEW blank import `_ "github.com/esalaine/envoy-go/test/fixtures/0021-http-ext-authz-grpc/inputs"` (alphabetical-after `0020`). NEW switch-case in the `BackendKind` dispatch for `HTTPExtAuthzGRPC` reusing the existing `startEchoBackend` helper + spawning an `extauthzgrpc.New(t)` instance per-test for the in-process gRPC auth server (scenarios 3+4 stop it before the request). ~+12 LoC. Task 9. |
| `test/fixtures/0021-http-ext-authz-grpc/` | NEW DIRECTORY | Differential fixture with 8 scenarios per SPEC §7.1. Plaintext-only topology: 1 echo-backend cluster + 1 auth-server gRPC cluster (with `http2_protocol_options: {}`) + 3 HCM listeners `l_test_a/b/c` (a: scenarios 1+2+5+6+7+8 with `failure_mode_allow:false`; b: scenario 3 with `failure_mode_allow:false` + `status_on_error:503`; c: scenario 4 with `failure_mode_allow:true` + `failure_mode_allow_header_add:true`). Three listeners separate scenarios 3+4's distinct `failure_mode_allow` values per the 18.1 SPEC §10 notable lesson (CheckSettings cannot override `failure_mode_allow`). |
| `test/fixtures/0021-http-ext-authz-grpc/envoy.yaml` | NEW | Reference Envoy bootstrap. 3 HCM listeners `l_test_a/b/c` (plaintext TCP; HCM chain `ext_authz → router`) each with listener-level `ExtAuthz` config (gRPC-mode: `grpc_service.envoy_grpc.cluster_name: c_authz_grpc`; `transport_api_version: V3`; `with_request_body` for scenario 5; per-listener `failure_mode_allow` / `status_on_error` as needed). Routes: `/scenario1` → c_backend; `/scenario2` → c_backend (auth denies); `/scenario3` (l_test_b); `/scenario4` (l_test_c); `/scenario5` (with_request_body); `/disabled` with per-route TPFC `ExtAuthzPerRoute{disabled: true}` (scenario 6); `/ctx` with per-route TPFC `ExtAuthzPerRoute{check_settings{context_extensions: {policy: "scenario7"}}}` (scenario 7); `/scenario8` (OkHttpResponse mutation). Cluster `c_backend` STRICT_DNS → echobackend subprocess. Cluster `c_authz_grpc` STRICT_DNS → extauthzgrpc subprocess with `typed_extension_protocol_options.envoy.extensions.upstreams.http.v3.HttpProtocolOptions.explicit_http_config.http2_protocol_options: {}` (mandatory for gRPC framing per the §11.P13 in-session scrape). ~200 LoC. Per SPEC §7.2. Task 11. |
| `test/fixtures/0021-http-ext-authz-grpc/envoy-go.yaml` | NEW | Equivalent envoy-go bootstrap. Same 3-listener topology + routes + per-route map; cluster type STATIC. ~200 LoC. Per SPEC §7.2. Task 11. |
| `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go` | NEW | Go driver issuing the 8 scenarios per SPEC §7.1 mirroring the phase-18.1 driver shape. Functions `runScenario1..runScenario8(ctx, baseURLs, authBaseURL) error` where `baseURLs` is a map of listener name → URL (l_test_a/b/c). Per-scenario assertion: byte-exact body (allow paths backend-echo verbatim; deny paths the auth service's verbatim DeniedHttpResponse.body) + response status equivalence + `/stats/prometheus` counter-delta equivalence on the 5 reachable counters + backend-arrival header assertions (upstream injection per OkHttpResponse mutation) + auth-server received-CheckRequest content assertions (scenario 7 — `context_extensions[policy] == "scenario7"`). **extauthzgrpc lifecycle helper** `setupAuthGRPC(t, ctx, port, scripts)`; teardown via `srv.Stop()`. **Counter-delta helper** `scrapeStats` + `assertCounterDelta` mirrors phase-18.1. The driver pre-populates the 8 scripted `CheckResponse` values via `srv.Script(":path-discriminator", resp)` before issuing requests. ~350 LoC. Per SPEC §7.1. Task 10. |
| `test/fixtures/0021-http-ext-authz-grpc/expectations.yaml` | NEW | Per-scenario allow-list + counter-delta map per SPEC §7. Documents the 8-scenario equivalence claim + the per-route 5th-canonical scenarios 6+7 + the divergence-window allow-list (`response_code_details` field ABSENT on the envoy-go side; `disabled` counter STRUCTURALLY UNREACHABLE — NOT asserted; cluster-scoped `cluster.*.ext_authz.*` triple not exercised; gRPC-specific deferred fields per SPEC §8: `initial_metadata`/`retry_policy`/`response_headers_to_add`/`query_parameters_to_*`/`dynamic_metadata*` silent-ignored; `OkResponse + non-zero-status` + `DeniedResponse + zero-status` → dispError envoy-go-strict per SPEC §6.7 — documented divergence-window from reference Envoy v1.37.2's lenient acceptance). ~85 LoC. Per SPEC §7. Task 12. |
| `test/fixtures/0021-http-ext-authz-grpc/README.md` | NEW | Fixture overview + 8-scenario list + reference-config citations + extauthzgrpc in-process gRPC server lifecycle notes + three-listener topology rationale (per the 18.1 SPEC §10 notable lesson — `CheckSettings` cannot override `failure_mode_allow`) + per-route 5th-canonical-REUSE discipline note (NO ADR-0125 amendment; ADR-0163 confirmed at 18.1) + SHARED-stats discipline + counter-delta assertion discipline + divergence-window note (`OkHttpResponse.response_headers_to_add` DEFERRED; `header_map` arm DEFERRED per D6; envoy-go-strict treatment of OkResponse+non-zero-status + DeniedResponse+zero-status as `dispError` per SPEC §6.7). ~130 LoC. Per SPEC §7.2. Task 12. |
| `docs/envoy-go/DECISIONS.md` | MODIFIED | **4 ADRs** anchored at 18.2 IMPL (5 if ADR-0165 fires — see Task 4): **ADR-0158** (`internal/grpcclient/` framework primitive; §Context already at parent SPEC commit `308e9b6` per ADR-0044; §Decision + §Consequences land at Task 3); **ADR-0157 §Decision AMENDMENT** in-place — replace the 18.1 "grpc_service arm PARSE-REJECTs in 18.1" wording with "grpc_service arm activates `buildGRPCCheckFn` at 18.2; GoogleGrpc PARSE-REJECTs envoy-go-strict; initial_metadata + retry_policy SILENT-IGNORED" (Task 3); **ADR-0160** gRPC-mode portion — §Decision + §Consequences extend the existing ADR-0160 entry with the `buildAttributeContext` body (Task 5); **ADR-0161** gRPC-mode portion — §Decision + §Consequences extend the existing ADR-0161 entry with the `mapGRPCResponse` + verbatim-deny-header pass-through + `OkHttpResponse.headers` append_action 4-arm dispatch (Task 6); **ADR-0165** (CONDITIONAL — fires if the callback-surface extension lands per D3; PLAN's strong hypothesis: YES, it fires) — anchors the 5 new `DecoderFilterCallbacks` methods (`DownstreamRemoteAddr`/`LocalAddr`/`TLSServerName`/`TLSPeerCertDER`/`Protocol`) + `ListenerPrincipal` as a cross-phase-reusable framework primitive (Task 4). ~+250 LoC (+90 if ADR-0165 fires). |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFIED | Per SPEC §13 — 8-edit bundle: **§13.1** flip the 18.1-anchored "gRPC mode — see phase 18.2" forward-pointers in the `### envoy.filters.http.ext_authz` subsection to ACTUAL gRPC content (services oneof → CONSUMED with grpc_service dispatch to `buildGRPCCheckFn`; NEW subsections on gRPC-mode `AttributeContext` populated set incl. the §11.P4 in-session findings + the `OkHttpResponse` mutation + `DeniedHttpResponse` verbatim pass-through CONTRAST with HTTP-mode + `GoogleGrpc` PARSE-REJECT + `initial_metadata`/`retry_policy` SILENT-IGNORED) ~120 LoC; **§13.2** stat-table 77 names UNCHANGED (the 6 ext_authz counters are mode-agnostic per SPEC §2.2); the planner adds a clarification headnote that 0 new counters land at 18.2 ~5 LoC; **§13.3** NEW Equivalence-Matrix row for fixture `0021-http-ext-authz-grpc` ~5 LoC; **§13.4** NEW `### Phase 18.2 forward-pointer notes` subsection covering the SPEC §8 12-item deferral list + the 2 closures from 18.1 (gRPC arm activation + `context_extensions` consumption) ~50 LoC; **§13.5** AMENDED in-place — flip the 18.1-anchored "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" to reflect the 5 new callback methods landed per D3 + ADR-0165; record the PLAN-time deviation rationale (the SPEC §13.5 hard constraint was in direct conflict with SPEC §15 item 4 populated-set requirement) ~20 LoC; **§13.6** ADR-0125 §(v) 5th-canonical-REUSE cross-reference UNCHANGED (ext_authz is already documented as the FIRST 5th-canonical-REUSE consumer at 18.1; gRPC-mode adds no per-route canonical change per SPEC §3); **§13.7** NEW top-level `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella — Documents: the `Dialer` API + connection-lifecycle (per-cluster cached `*grpc.ClientConn` via `grpc.WithContextDialer(cluster.Dial)` — TLS terminates at the cluster-manager layer); the `AuthClient` typed wrapper; cross-phase reuse intent for ext_proc + global_ratelimit; the §11.P13 in-session RATIFICATION note (no new TLS-layer lift) ~80 LoC; **§13.8** the existing `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)` UNCHANGED (gRPC client is a separate primitive per ADR-0158; HTTP-outbound stays at 18.1's ADR-0159). Total ~+280 LoC. Task 13. |
| `docs/envoy-go/ROADMAP.md` | MODIFIED | Row `18.2` status `in-progress → done` + summary sharpening (post-impl counts: 14-task PLAN-confirmed + ~885–1485 LoC production estimate + final 4–5-ADR roster anchored). **AT THE SAME COMMIT:** row `18` status `in-progress → done` per parent SPEC §8 parent-rollup discipline. The commit-message body MUST explicitly name BOTH transitions for grep-verifiability (per SPEC §15 item 13). ~+2 net. Task 13. |
| `docs/envoy-go/STATE.md` | MODIFIED | Advance per `BOOTSTRAP_PROMPT.md` §5 lifecycle ~rewrite-in-place. Final state: `active-phase: <next>` (TBD at Task 13 against the BOOTSTRAP §5 lifecycle-state machine — the next §9 family-row is numbered `19`); `lifecycle-state: phase 18.2 done; phase 18 done; phase 19 BRAINSTORM pending` (or analog); `next-skill:` the BRAINSTORM skill for phase 19; `last-commit: <Task 13 squash>`; `next-free ADR: ADR-0166` (or `ADR-0165` if the ADR-0044 escape-valve did NOT fire — but the PLAN's strong hypothesis is that it DOES fire at Task 4); `last-updated: <impl-date>`. Task 13. |
| `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` | NEW | Lifecycle artefact. Append-only log; each task lands one entry. Quote command outputs verbatim. Mirror the phase-09..18.1 PROGRESS.md structure. ~600 LoC across 14 task entries. |
| `docs/envoy-go/phases/18.2-ext-authz-grpc/REVIEW.md` | NEW | Lifecycle artefact. End-of-phase review per `superpowers:requesting-code-review`. ~240 LoC. Task 14. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by SPEC §12 to settle the SPEC's seven deferred decisions before implementation; this PLAN settles all seven plus a handful that emerged at PLAN-drafting time. The resolutions are recorded in `PROGRESS.md`'s preamble (Task 1) and reproduced here so the implementer at each task can act without re-deriving them:

1. **D1 — `test/helpers/extauthzgrpc/` discriminator + helper API LOCKED per SPEC §7.4 + §12 item 1.** Script discriminator: the `:path` value extracted from `req.Attributes.Request.Http.Path`. API surface per SPEC §7.4 sketch: `New(t testing.TB) *Server` returning a started server bound to `127.0.0.1:0`; `(*Server).Addr() string`; `(*Server).Script(path string, resp *authv3.CheckResponse)`; `(*Server).Stop()`. Lifecycle: spawn-per-fixture via `t.Cleanup(s.Stop)`. Plaintext h2c (no TLS) — fixture 0021 uses a plaintext auth cluster; TLS-to-auth coverage stays unit-test-only per SPEC §7.2 known-testing-gap. *Anchored: SPEC §7.4 + §12 item 1.*

2. **D2 — `*grpc.ClientConn` close-on-process-exit discipline LOCKED at MVP leaks-on-exit per SPEC §12 item 2.** No `os.Exit` cleanup hook; no `cleanup` package registration. The `*grpc.ClientConn` is owned by the `*compiledConfig` (captured by the `checkFn` closure); on process exit, the OS reclaims the connection. Rationale: matches 18.1's `httpAuthClient` no-shutdown discipline; envoy-go has no config hot-reload yet (xDS-CDS deferred per SPEC §8 item 9); the per-(cluster, compiledConfig) ClientConn lifetime is process-bounded. A future hot-reload phase will land a close-on-replacement discipline per a new ADR (NOT 18.2). *Anchored: SPEC §3.1 + §12 item 2.*

3. **D3 — `*authRequest` extension + per-stream-state seeding LOCKED at extend-`*authRequest` + extend-`DecoderFilterCallbacks` per SPEC §6.5 step 5 + §12 item 3.** Extend the existing 18.1 `*authRequest` struct (in `extauthz.go`) with: `remoteAddr net.Addr`, `localAddr net.Addr`, `tlsServerName string`, `peerCertDER []byte`, `listenerPrincipal string`, `protocol string`, `requestID string`, `streamStartTime time.Time`, `perRouteContextExtensions map[string]string`, `downstreamPrincipal []string`. The 18.1 fields (`method`/`path`/`headers`/`body`) carry forward unchanged. The closure signature `(ctx, *authRequest)` stays mode-agnostic per ADR-0157 §Decision. Per-stream-state SOURCE: NEW callback methods on `DecoderFilterCallbacks` (5 new: `DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`) + `ListenerPrincipal` (also new). These are seeded at HCM-dispatch time onto the per-stream `*FilterChain` via 6 new chain primitives mirroring the existing `SetTLSPrincipals` / `tlsPrincipals` / `(*decoderCB) DownstreamPrincipal` pattern (chain.go:107 + 551 + 483). **D3-DEVIATION-FROM-SPEC §13.5:** SPEC §13.5 stated "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; the planner verified at PLAN-time that the SPEC's hard constraint is in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION (populated `tls_session.sni`, `source.certificate`, socket addresses, `destination.principal`); the SPEC's "extracted from connection state at `DecodeHeaders` time when the per-stream `dcb` is in scope" requirement is unsatisfiable without callback extension (master tip's `internal/filter/http/callbacks.go` exposes only `DownstreamPrincipal()` for TLS-aware state — verified by reading the file). The PLAN settles by AMENDING SPEC §13.5 at Task 4 — the callback-surface extension is unavoidable; the alternative (UNPOPULATED fields) is a behaviorally significant divergence vs reference Envoy and contradicts SPEC §15 item 4. **ADR-0044 escape-valve fires: ADR-0165 lands at Task 4** anchoring the callback-group as a cross-phase-reusable framework primitive (ext_proc + global_ratelimit + future ext_authz extensions). *Anchored: SPEC §6.5 step 5 + §12 item 3 + §13.5 (AMENDED at Task 4) + SPEC §15 item 4 + §11.P4.*

4. **D4 — `grpc.NewClient` resolver target string LOCKED at `passthrough:///<cluster_name>` per SPEC §6.5 step 3 + §12 item 4.** The `passthrough:///` scheme is gRPC's built-in single-endpoint resolver; it skips DNS resolution and delegates endpoint selection to our `WithContextDialer` callback (which re-looks-up via `cluster.Manager.Get(cluster_name).Dial(ctx)`). Functionally equivalent to `dns:///` for this use case but cleaner — gRPC doesn't try to be smart about resolution; we own it via the cluster manager. The cluster name is embedded in the target URL for clean logging (gRPC logs the target string on failures). *Anchored: SPEC §6.5 step 3 + §12 item 4.*

5. **D5 — `*core.HeaderValueOption.append_action` 4-arm dispatch table LOCKED per SPEC §6.7 + §12 item 5.** The four enum values: `APPEND_IF_EXISTS_OR_ADD` (default; index 0) → `upstreamApp` (append-discipline; `applyUpstreamMutations` step 2 via `headers.Add`); `OVERWRITE_IF_EXISTS_OR_ADD` (index 1) → `upstreamSet` (overwrite-discipline; `applyUpstreamMutations` step 1 via `headers.Set`); `OVERWRITE_IF_EXISTS` (index 2) → `upstreamSet` BUT WITH `setIfAbsent: false` semantic — only overwrites if the header is already present, does NOT add (the 4-arm dispatch's `OVERWRITE_IF_EXISTS` distinct branch); the IMPL extends `headerKV` with a `setIfAbsent` discriminator (default `true` for `OVERWRITE_IF_EXISTS_OR_ADD`; `false` for `OVERWRITE_IF_EXISTS`) — `applyUpstreamMutations` checks `len(headers.Values(name)) > 0` before `Set` when `setIfAbsent: false`; `ADD_IF_ABSENT` (index 3) → `upstreamSet` BUT WITH `setIfAbsent: true` AND `addOnlyIfNotPresent: true` — adds only when the header is absent (does NOT overwrite); the IMPL extends `headerKV` further with an `addOnlyIfAbsent` discriminator (default `false`; `true` for `ADD_IF_ABSENT`) — `applyUpstreamMutations` checks `len(headers.Values(name)) == 0` before `Set` when `addOnlyIfAbsent: true`. Phase-10 header_mutation enum-handling precedent is the model. The unit-test Group 11 covers all 4 arms. **Implementation note:** the IMPL may collapse the two new discriminators into a single 4-value enum field on `headerKV` (cleaner than two booleans). The IMPL settles the exact representation; behavior is the same. *Anchored: SPEC §6.7 + §12 item 5 + phase-10 header_mutation precedent.*

6. **D6 — `encode_raw_headers` `header_map` arm activation LOCKED at DEFERRED per SPEC §8 item 8 + §12 item 6.** When `encode_raw_headers: true`, envoy-go's `buildAttributeContext` does NOT populate `request.http.header_map` (the `core.HeaderMap` field preserving header order); only the legacy `request.http.headers` map is populated. Rationale: the §11.P4 in-session SPEC scrape evidence shows reference Envoy populates `headers` by default and only switches to `header_map` when `encode_raw_headers: true`; fixture 0021's byte-equivalence assertion compares the auth-server's received-CheckRequest against expectations (the harness's protojson rendering treats `headers` as the canonical form when both fields would otherwise serialize differently); the cost of implementing `header_map` (preserving header order through `http.Header → map[string]string` conversion is lossy by default) outweighs the MVP benefit. The flag PARSES (no PARSE-REJECT) — operators setting it true see the legacy `headers` map populated as if the flag were false. Divergence-window documented in BEHAVIOR_CONTRACT §13.4. *Anchored: SPEC §8 item 8 + §12 item 6 + §11.P4 in-session evidence.*

7. **D7 — gRPC transport-error vs `CheckResponse.status.code` non-zero distinction LOCKED per SPEC §6.7 + §12 item 7.** Transport-level errors (gRPC `Unavailable` / `DeadlineExceeded` / `Canceled` from `*grpc.ClientConn.Invoke`; `context.Canceled` from `OnDestroy`-cancellation; `context.DeadlineExceeded` from per-Check timeout) surface as the `error` return of `(*AuthClient).Check` — `mapGRPCResponse` is NEVER called on a transport-error path; the closure body explicitly returns `(checkDisposition{class: dispError}, err)` on `err != nil` BEFORE calling `mapGRPCResponse`. `CheckResponse.status.code` non-zero values (any gRPC canonical code: `PERMISSION_DENIED` / `UNAUTHENTICATED` / `INVALID_ARGUMENT` / etc.) → handled BY `mapGRPCResponse` per the §6.7 truth table: with `DeniedResponse` → dispDeny; with `OkResponse` → dispError (envoy-go-strict — structurally inconsistent); with nil-oneof → dispDeny default 403. This cleanly separates the transport-error path (handled at the `AuthClient.Check` boundary; the filter's closure body) from the proto-message-content path (handled in `mapGRPCResponse`). *Anchored: SPEC §6.7 + §12 item 7 + parent §5.P10.*

8. **D8 — `extauthz_test.go` single-file LOCKED per the 18.1 D3 precedent (NEW; surfaces at PLAN-time).** All Groups 10–14 stay in one `extauthz_test.go` for 18.2 (mirrors the 18.1 single-file discipline; 18.1's file is ~4900 LoC and stayed in one file — the soft threshold is ~5000 LoC before a split becomes mandatory). Impl-time MAY split `gRPC_test.go` if the combined file exceeds ~6000 LoC. *Anchored: 18.1 PLAN D3 precedent.*

9. **D9 — gRPC `Authorization/Check` deadline propagation discipline LOCKED at per-Check `context.WithTimeout` per SPEC §6.5 + §14.2 + planner-time emerge.** The `*AuthClient.Check(ctx, req)` method applies `ctx, cancel := context.WithTimeout(callerCtx, timeout)` where `timeout` is the `*HttpService.server_uri.timeout`-analog for gRPC mode (`gs.Timeout` from `*GrpcService`); the cancel is deferred. The caller's `ctx` (from the filter's `dispatchOutboundCheck` async goroutine) is the parent — its cancellation (from `OnDestroy` via `callCancel()`) propagates through `context.WithTimeout`'s internal AND-of-cancellation semantics. Result: BOTH `OnDestroy`-cancellation AND per-Check timeout surface as transport errors via `err != nil` from `(*AuthClient).Check`. NO ADR escape-valve for this surface — the standard `context.WithTimeout` semantics suffice. *Anchored: SPEC §6.5 + §14.2 + planner-time clarification of §3.1 timeout-application-site.*

10. **D10 — Three-listener fixture topology LOCKED per SPEC §7.2 (NEW; surfaces at PLAN-time).** Fixture 0021 wires 3 HCM listeners `l_test_a/b/c` to separate scenarios with distinct `failure_mode_allow` values (per the 18.1 SPEC §10 notable lesson — `CheckSettings` cannot override `failure_mode_allow`, so a single listener cannot host both `failure_mode_allow:false` AND `failure_mode_allow:true` scenarios). `l_test_a` hosts scenarios 1/2/5/6/7/8 (`failure_mode_allow:false`; `status_on_error:503` UNREACHABLE on these scenarios); `l_test_b` hosts scenario 3 (`failure_mode_allow:false` + `status_on_error:503` reachable via auth-server-down setup); `l_test_c` hosts scenario 4 (`failure_mode_allow:true` + `failure_mode_allow_header_add:true`). Each listener routes to a per-scenario `:path` route; the auth-server-down scenarios (3+4) stop the in-process gRPC server BEFORE the request issues (the driver's `setupAuthGRPC` helper) — mirrors the 18.1 fixture-0020 auth-down treatment. *Anchored: SPEC §7.2 + 18.1 SPEC §10 lesson.*

11. **D11 — `OkHttpResponse.response_headers_to_add` DEFERRED behavior LOCKED at SILENT-IGNORED per SPEC §8 item 5 (NEW; surfaces at PLAN-time).** The field PARSES; envoy-go does NOT inject these headers into the downstream RESPONSE on allow (the filter is decoder-only per ADR-0156; no encode-leg). The fuzz corpus + Group 11 unit test cover the silent-ignore path. Documented in BEHAVIOR_CONTRACT §13.1 + §13.4 as a divergence-window joint with the 18.1 `allowed_client_headers_on_success` deferral. *Anchored: SPEC §8 item 5.*

12. **D12 — ADR-0044 escape-valve disposition: PLAN-time HYPOTHESIS that ADR-0165 fires at Task 4 (NEW; surfaces at PLAN-time).** Per the planner-time settle of D3 (callback-surface extension is unavoidable to satisfy SPEC §15 item 4 + §11.P4 populated-set RATIFICATION), the IMPL anchors **ADR-0165** at Task 4 as the cross-phase-reusable framework primitive for the 5 new `DecoderFilterCallbacks` methods (`DownstreamRemoteAddr` / `DownstreamLocalAddr` / `DownstreamTLSServerName` / `DownstreamTLSPeerCertDER` / `DownstreamProtocol`) + `ListenerPrincipal`. The SPEC §10 anticipated "~0–1 impl-time-unanticipated ADRs"; ADR-0165 lands as 1. If at IMPL time the implementer finds an alternative path that avoids the callback extension (unlikely — the planner verified the SPEC's required population set against the master-tip callback surface), the PLAN's D3 + D12 settle reverts and ADR-0165 does NOT land. The IMPL records the outcome in PROGRESS.md Task 4. Next-free-ADR after 18.2: `ADR-0166` (if ADR-0165 fires) or `ADR-0165` (if it does not). *Anchored: SPEC §10 + D3.*

13. **D13 — Fixture 0021 IS plaintext-only — NO PKI, NO TLS-to-auth fixture coverage (NEW; surfaces at PLAN-time).** Unlike phase-17's RSA/ECDSA PKI fixture or phase-16's mTLS fixture, fixture 0021 wires plaintext HTTP/1.1 listeners + plaintext h2c auth cluster. The §11.P13 in-session SPEC scrape RATIFIED the TLS-to-auth-cluster path against reference Envoy; behavioral verification of envoy-go's own TLS handshake against the gRPC auth cluster lives in `internal/grpcclient/grpcclient_test.go` Group 1 (unit test against a TLS-fronted test gRPC server) per SPEC §14.1; AttributeContext-side TLS-aware fields (`tls_session.sni`, `source.certificate`, `destination.principal`) are unit-tested against MOCKED `*authRequest` state per SPEC §7.2 known-testing-gap. A future integration test MAY close the differential gap if a behavior delta surfaces; the current scope DEFERS this per the cost-vs-coverage tradeoff (the §11.P13 RATIFICATION is the load-bearing empirical evidence). *Anchored: SPEC §7.2 + §11.P13.*

---

## ADRs introduced by this plan

The 18.2-landing ADRs anticipated by SPEC §10 (ADR-0158 §Decision + §Consequences; ADR-0157 §Decision AMENDMENT; ADR-0160 gRPC-mode portion §Decision + §Consequences; ADR-0161 gRPC-mode portion §Decision + §Consequences) **plus 1 conditional impl-time-unanticipated ADR** (ADR-0165 — the callback-surface extension framework primitive per D3 + D12; the PLAN's strong hypothesis: it fires at Task 4). **§Context drafts for ADR-0158/0160/0161** were already landed at the parent SPEC commit `308e9b6` per ADR-0044 ADR-on-impl convention; **ADR-0157's full §Decision was at 18.1** — the 18.2 IMPL AMENDS in-place. **ADR-0164** (the ADR-0045 split-application ADR) landed IN FULL at the parent SPEC commit — UNCHANGED by 18.2 IMPL.

| ADR | Subject (18.2 portion) | Lands-in-task |
|---|---|---|
| **ADR-0158** | `internal/grpcclient/` framework primitive — `Dialer` (cluster-name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` — TLS terminates at the cluster-manager layer per the §11.P13 in-session SPEC scrape) + `AuthClient` typed wrapper (`envoy.service.auth.v3.Authorization/Check` stub from `go-control-plane v1.32.4` — no codegen); one `*grpc.ClientConn` per (cluster_name, compiledConfig) pair created at config-load time + shared across per-stream Check calls; leaks-on-exit MVP per D2; per-Check `context.WithTimeout` per D9; cross-phase-reusable for ext_proc + global_ratelimit per ADR-0158 §Consequences | Task 3 |
| **ADR-0157 §Decision AMENDMENT** | `*ExtAuthz_GrpcService` switch-arm activation in `buildCompiledConfig` — replaces the 18.1 PARSE-REJECT with `buildGRPCCheckFn`; `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict (`"ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"`); `core.GrpcService.initial_metadata` + `retry_policy` SILENT-IGNORED; `compiledConfig` struct shape UNCHANGED (field-final at 18.1) — gRPC-specific config captured in closure lexical scope per §6.5 step 5 | Task 3 |
| **ADR-0160** (gRPC-mode portion) | `buildAttributeContext` in `attributes.go` — source/destination `Peer` (incl. `principal` via ADR-0144); `request.http` per parent §5.P4 + §11.P4 in-session refinement (pseudo-headers lowercased + included in headers map; HCM-injected `x-forwarded-proto`/`x-request-id`/`x-envoy-auth-partial-body` visible by the time DecodeHeaders runs); `request.time` as `Timestamp`; `tls_session.sni` gated by `include_tls_session` (per §11.P4 RATIFICATION — ONLY `sni` populated); `source.certificate` gated by `include_peer_certificate` (DER-encoded leaf); `destination.principal` populated AUTOMATICALLY from listener TLS cert per §11.P4 (NOT gated); `context_extensions` merged listener+per-route; `encode_raw_headers` `header_map` arm DEFERRED per D6; `metadata_context` + `route_metadata_context` populated as empty messages | Task 5 |
| **ADR-0161** (gRPC-mode portion) | `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` in `check.go`; `OkHttpResponse.headers` set/append per the 4-arm `append_action` dispatch table per D5; `OkHttpResponse.headers_to_remove` populated into new `upstreamDel []string` field on `checkDisposition`; `OkHttpResponse.response_headers_to_add` SILENT-IGNORED per D11 (decode-side-only filter shape); `DeniedHttpResponse.{status.code, body, headers}` extracted verbatim (NOT filtered through `allowed_client_headers` — UNLIKE HTTP-mode; per parent §5.P11); envoy-go-strict treatment of `OkResponse + non-zero status` AND `DeniedResponse + zero status` as `dispError` per SPEC §6.7 commentary; `validate_mutations` gating identical to HTTP-mode → `dispInvalid` + `invalid` counter | Task 6 |
| **ADR-0165** (CONDITIONAL — fires per D3 + D12) | Cross-phase-reusable callback-surface extension to `DecoderFilterCallbacks` — adds 5 new accessor methods (`DownstreamRemoteAddr()`, `DownstreamLocalAddr()`, `DownstreamTLSServerName()`, `DownstreamTLSPeerCertDER()`, `DownstreamProtocol()`) + `ListenerPrincipal()` for per-stream socket + TLS + listener-cert state needed by ext_authz gRPC-mode's `AttributeContext` builder; seeded at HCM-dispatch (H1 `connection.go` + H2 `h2dispatch.go`) via 6 new chain primitives mirroring the `SetTLSPrincipals` / `tlsPrincipals` / `DownstreamPrincipal()` pattern. Anchors the PLAN-time settle of D3 (the SPEC §13.5 hard "NO new method" constraint is in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION — the callback extension is unavoidable). §Context + §Decision + §Consequences ALL land at Task 4 (no pre-anchored §Context — the ADR is impl-time-unanticipated at SPEC time per ADR-0044). | Task 4 (CONDITIONAL) |

The implementer at each impl-anchor task AUTHORS the ADR §Decision + §Consequences bodies in DECISIONS.md (for ADR-0158: in the slot of the existing §Context-draft; for ADR-0157: in-place AMENDMENT of the existing §Decision; for ADR-0160/0161: in-place EXTENSION of the existing §Decision + §Consequences with the gRPC-mode portion; for ADR-0165: a fresh ADR entry inserted before "ADR tail" with `Status: Accepted` + `Date: <impl-date>` + `Lands-in: Task 4 of phase-18.2`), includes the ADR in the commit message, and verifies via `grep -nE '^## ADR-XXXX' docs/envoy-go/DECISIONS.md` returning the expected match count.

**NO in-place ADR-0125 amendment required by phase 18.2** (5th-canonical-REUSE already recorded at 18.1 via ADR-0163; no new canonical added).

**ADR-0044 escape-valve fires at Task 4 per D3 + D12** — `ADR-0165` lands. If at IMPL time the implementer finds an alternative path that avoids the callback extension (highly unlikely — see D3 rationale), ADR-0165 does NOT land + the PLAN's D12 hypothesis is recorded as falsified in PROGRESS.md.

---

## Execution preconditions

Before Task 1 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-18.2-ext-authz-grpc-impl \
                 -b phase-18.2-ext-authz-grpc-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-18.2-ext-authz-grpc-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 17 preconditions verified at Task 1 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-18.2-ext-authz-grpc-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -6` shows the 18.2-PLAN.md squash commit + its SHA-fill follow-up at the head, with the 18.2-SPEC.md squash commit `729867e` + its SHA-fill follow-up `be18857` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `164` (ADR-0164 — the highest ADR anchored as of master tip). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0158' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0158 §Context already at parent SPEC commit per ADR-0044). `grep -cE '^## ADR-016[01]' docs/envoy-go/DECISIONS.md` returns `2` (ADR-0160 + ADR-0161 already have HTTP-mode portions filled at 18.1; the gRPC-mode portion extends them in-place). `grep -nE '^## ADR-0165' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0165 fires at Task 4 if D12 hypothesis holds).
6. **NO ADR-0125 §(xiv) amendment.** `grep -nE '\(xiv\)' docs/envoy-go/DECISIONS.md` returns 0 matches — phase 18 lands NO ADR-0125 amendment (ADR-0163 records the explicit no-amendment 5th-canonical-REUSE decision; 18.2 changes nothing about the per-route discipline). If `(xiv)` returns ≥1, investigate before proceeding.
7. **SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` returns `729867e` (or descendant). If different, re-read SPEC.
8. **PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
9. **Pristine tree.** `git status --porcelain` returns empty.
10. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
11. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|20)'` returns every fixture 0000–0020 PASS — the 21 pre-existing fixtures are the regression baseline. Phase 18.2 adds the 22nd (`0021-http-ext-authz-grpc` per Task 12).
12. **Pre-existing fuzzers run clean at 30s.** The 22 fuzzers from phases 02–18.1 run clean. Phase 18.2 adds the 23rd (`FuzzCheckResponseMapping` per Task 9).
13. **Reference Envoy image present.** `docker image inspect envoyproxy/envoy:v1.37.2` returns the SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 pin; unchanged).
14. **`google.golang.org/grpc` v1.70.0 reachable.** `go list -m google.golang.org/grpc` returns `google.golang.org/grpc v1.70.0` (currently an INDIRECT dep at master tip; Task 2 PROMOTES to direct). `go doc google.golang.org/grpc NewClient | head -5` returns the `NewClient` function signature.
15. **`envoy.service.auth.v3` proto package reachable.** `go doc github.com/envoyproxy/go-control-plane/envoy/service/auth/v3 AuthorizationClient | head -5` returns the `AuthorizationClient` interface without an `import path failed` error. If it fails, `go mod download`.
16. **Pre-existing `internal/grpcclient/` directory does NOT exist.** `test ! -d internal/grpcclient && echo "ok: grpcclient absent"` returns success.
17. **Pre-existing `test/helpers/extauthzgrpc/` directory does NOT exist.** `test ! -d test/helpers/extauthzgrpc && echo "ok: extauthzgrpc absent"` returns success.

If all 17 preconditions pass, proceed to Task 1.

---

## Task 1: Execution-precondition check + PROGRESS.md preamble

**Files:**
- Create: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0158's §Context draft is at the SPEC commit; ADR-0157's full body is at 18.1; ADR-0160/0161's HTTP-mode portion bodies are at 18.1; ADR-0165 is CONDITIONAL (PLAN hypothesis: it fires at Task 4). The PROGRESS preamble ANTICIPATES the 4–5 ADRs (each with its Lands-in-task anchor reproduced from this PLAN's per-ADR table) and records the 13 planner-time decisions.

**Precondition:** worktree exists at `phase-18.2-ext-authz-grpc-impl`; branch base is master tip after the PLAN.md SHA-fill follow-up; all 17 preconditions report green.
**Artifact:** `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` returns the Task 1 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` with: (a) Preamble summarizing the 17-precondition verification (verbatim command outputs captured); (b) the 4–5-ADR table from `## ADRs introduced by this plan` reproduced verbatim; (c) the 13 planner-time decisions reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Task 1 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 1: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
# expect: a 40-char SHA (Task 1 commit)
```

---

## Task 2: `internal/grpcclient/` skeleton — `doc.go` + `grpcclient.go` (types + signatures + nil-method bodies) + `grpcclient_test.go` Groups 1+2 skeleton

**Files:**
- Create: `internal/grpcclient/doc.go`
- Create: `internal/grpcclient/grpcclient.go`
- Create: `internal/grpcclient/grpcclient_test.go`
- Modify: `go.mod` (promote `google.golang.org/grpc` from indirect to direct dep)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 2 entry)

Establishes the `internal/grpcclient/` package skeleton: the `Dialer`/`AuthClient` types, signatures for `New`, `DialContext`, `NewAuthClient`, `Check`, `Close` — method bodies STUBBED with sentinel errors. The real method bodies land at Task 3. `go.mod` promotes `google.golang.org/grpc` to a direct dep (currently INDIRECT at master tip). The skeleton lets later tasks reference the types without forward declarations + lets Task 3's TDD-style tests build against the stubs first.

**Precondition:** Task 1 acceptance green.
**Artifact:** new `internal/grpcclient/` directory with `doc.go` + `grpcclient.go` + `grpcclient_test.go`; `go.mod` updated.
**Acceptance:** `go build ./internal/grpcclient/...` exit 0; `go vet ./internal/grpcclient/...` exit 0; Groups 1+2 skeleton tests compile (FAIL — stubs return sentinel errors); `go mod tidy` does not break.

- [ ] **Step 1: Author `doc.go`** — package overview per the File structure table responsibility for `internal/grpcclient/doc.go`.

- [ ] **Step 2: Author `grpcclient.go` skeleton** — the `Dialer` + `AuthClient` types + signature stubs. Stub bodies return `errors.New("grpcclient: TODO (Task 3)")`. Include the `passthrough:///` import comments + `WithContextDialer` integration comments per SPEC §3.1 + planner-time decision D4 for the Task 3 implementer.

- [ ] **Step 3: Author `grpcclient_test.go` Groups 1+2 test scaffolding** — table-driven tests for `Dialer.DialContext` happy/PARSE-REJECT paths + `AuthClient.Check` happy/timeout/cancel paths. The test SCAFFOLDING is fully written (table cases enumerated); the assertions match the eventual real impl. Tests FAIL against the stubs (sentinel error returned where success is expected).

- [ ] **Step 4: Promote `google.golang.org/grpc` to direct** — edit `go.mod` to add `google.golang.org/grpc v1.70.0` (without the `// indirect` comment). Run `go mod tidy`; verify `go list -m google.golang.org/grpc` returns the version without `(indirect)`.

- [ ] **Step 5: Verify build + vet** — `go build ./internal/grpcclient/... && go vet ./internal/grpcclient/...` exit 0; Groups 1+2 tests FAIL (expected — stubs).

- [ ] **Step 6: Commit**

```bash
git add internal/grpcclient/ go.mod go.sum docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 2: internal/grpcclient/ skeleton — Dialer + AuthClient types + signatures + Groups 1+2 test scaffolding"
```

---

## Task 3: `internal/grpcclient/grpcclient.go` real impl — `DialContext` + `NewAuthClient` + `Check` + `Close` + Groups 1+2+3 tests pass [ADR-0158] + ADR-0157 §Decision AMENDMENT (grpc_service arm activation)

**Files:**
- Modify: `internal/grpcclient/grpcclient.go` (real method bodies)
- Modify: `internal/grpcclient/grpcclient_test.go` (extend Group 3 — Close idempotency)
- Modify: `internal/filter/http/extauthz/extauthz.go` (`buildCompiledConfig` `*ExtAuthz_GrpcService` arm: replace the 18.1 PARSE-REJECT with `buildGRPCCheckFn` call — Task 3 lands the WIRE-UP; the `buildGRPCCheckFn` body itself lands at Task 5/6; for Task 3 the call site is wired with the real `buildGRPCCheckFn` from `check.go` whose body STUBS the gRPC arms returning a sentinel error — same skeleton-then-fill pattern as 18.1 Task 2)
- Create: `internal/filter/http/extauthz/check.go` extension (NEW `buildGRPCCheckFn` stub returning sentinel — body lands at Task 5)
- Modify: `docs/envoy-go/DECISIONS.md` (fill in ADR-0158 §Decision + §Consequences; AMEND ADR-0157 §Decision in-place)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 3 entry)

Lands the real `internal/grpcclient/` method bodies + the `grpc_service` switch-arm activation in `buildCompiledConfig`. `Dialer.DialContext` builds a `*grpc.ClientConn` via `grpc.NewClient("passthrough:///"+clusterName, grpc.WithContextDialer(...), grpc.WithTransportCredentials(insecure.NewCredentials()))` per SPEC §3.1 + planner-time decision D4. The `WithContextDialer` callback re-looks-up the cluster via `mgr.Get(clusterName)` and calls `cluster.Dial(ctx)`. PARSE-REJECTs (via error return): unknown cluster name, `UseH2()==false`. `NewAuthClient` constructs an `AuthClient` wrapping the dialed `ClientConn` + the typed `authv3.AuthorizationClient` stub + the per-Check timeout. `Check(ctx, req)` applies `context.WithTimeout(ctx, timeout)` and calls `stub.Check(timedCtx, req)`; transport errors propagate verbatim per planner-time decision D7. `Close()` is idempotent via a sync.Once guard. The `buildCompiledConfig` `*ExtAuthz_GrpcService` arm flips from the 18.1 PARSE-REJECT to a call into `buildGRPCCheckFn(s.GrpcService, ctx, ...)` (the body is STUBBED here — the real body lands at Task 5/6); the `buildGRPCCheckFn` STUB returns a sentinel error so existing 18.1 tests that PARSE-REJECT on `grpc_service` configs continue to fail with the new error wording — those tests get updated at Task 5. **ADR-0158 §Decision + §Consequences land here (the `internal/grpcclient/` primitive); ADR-0157 §Decision AMENDMENT lands here (the `grpc_service` arm activation in `buildCompiledConfig` — flip the error wording + the switch-arm body).**

**Precondition:** Task 2 acceptance green.
**Artifact:** real `internal/grpcclient/` implementation; `buildCompiledConfig` arm flipped; ADR-0158 + ADR-0157 AMENDMENT in DECISIONS.md.
**Acceptance:** Groups 1+2+3 in `grpcclient_test.go` PASS; `go test -race -count=1 ./internal/grpcclient/...` exit 0; `go vet ./...` exit 0; `grep -nE '^## ADR-0158' docs/envoy-go/DECISIONS.md` returns 1 match (with §Decision + §Consequences filled). The 18.1-anchored "grpc_service mode not yet supported (lands in phase 18.2)" PARSE-REJECT error no longer fires from `buildCompiledConfig` (verified via existing 18.1 Group 1 test cases — they will FAIL with a different error since `buildGRPCCheckFn` STUB returns a different sentinel; the failing tests are documented as expected at this task + fixed at Task 5).

- [ ] **Step 1: Run Groups 1+2+3 tests to verify they FAIL** — `go test ./internal/grpcclient/ -v` expects all FAIL with the Task-2 sentinel error.

- [ ] **Step 2: Implement `Dialer.New` + `DialContext`** — per SPEC §3.1 + D4. The `WithContextDialer` callback: `func(ctx context.Context, _ string) (net.Conn, error) { c, _, err := clu.Dial(ctx); return c, err }`. Use `insecure.NewCredentials()` for `WithTransportCredentials`. The `passthrough:///<clusterName>` target URL.

- [ ] **Step 3: Implement `NewAuthClient` + `Check` + `Close`** — `Check` applies `context.WithTimeout(ctx, timeout)` + defers cancel + calls `stub.Check(timedCtx, req)`; transport errors return verbatim per D7. `Close` uses `sync.Once` for idempotency per D9.

- [ ] **Step 4: Wire the `*ExtAuthz_GrpcService` arm in `buildCompiledConfig`** — replace the 18.1 PARSE-REJECT with `buildGRPCCheckFn(s.GrpcService, ctx, cc.validateMutations, raw.GetIncludePeerCertificate(), raw.GetIncludeTlsSession(), raw.GetEncodeRawHeaders(), packAsBytesFromWRB(cc.withRequestBody))`. Author the `buildGRPCCheckFn` STUB in `check.go` (returns `nil, errors.New("ext_authz: grpc_service: TODO (Task 5)")`).

- [ ] **Step 5: Run Groups 1+2+3 tests to verify they PASS** — `go test -race -count=1 ./internal/grpcclient/...` exit 0.

- [ ] **Step 6: Fill in ADR-0158 §Decision + §Consequences** — `Status: Accepted`, `Date: <impl-date>`, `Lands-in: Task 3 of phase-18.2`. Body content per the ADR table at `## ADRs introduced by this plan`. Include the `passthrough:///` rationale + the `WithTransportCredentials(insecure)` rationale + the cross-phase reuse forward-pointer (ext_proc + global_ratelimit).

- [ ] **Step 7: AMEND ADR-0157 §Decision in-place** — replace the 18.1 wording about "grpc_service arm PARSE-REJECTs in 18.1; §Decision amended at 18.2 IMPL" with the post-AMENDMENT wording: "grpc_service arm activates `buildGRPCCheckFn` at 18.2; GoogleGrpc arm PARSE-REJECTs envoy-go-strict per §4.3 + ADR-0008; initial_metadata + retry_policy SILENT-IGNORED per §2.6 + §8 items 2+3". Add `Date: <impl-date>` note to the §Decision header. Keep all other ADR-0157 content (the error-classification boundary, the 18.1 portion) UNCHANGED.

- [ ] **Step 8: Commit**

```bash
git add internal/grpcclient/ internal/filter/http/extauthz/extauthz.go internal/filter/http/extauthz/check.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 3: internal/grpcclient/ real impl + Groups 1+2+3 PASS + grpc_service arm activation [ADR-0158, ADR-0157 §Decision AMENDMENT]"
grep -nE '^## ADR-0158' docs/envoy-go/DECISIONS.md
# expect: 1 match
```

---

## Task 4: Callback-surface extension — 5 new `DecoderFilterCallbacks` methods + chain seeding primitives + HCM-dispatch wire-in + Group 13 [ADR-0165 — ADR-0044 escape-valve fires per D3 + D12]

**Files:**
- Modify: `internal/filter/http/callbacks.go` (5 new methods on `DecoderFilterCallbacks`)
- Modify: `internal/filter/http/chain.go` (5 new chain fields + 5 chain seeding primitives + 5 `*decoderCB` reader methods)
- Modify: `internal/filter/http/chain_test.go` (5 new round-trip tests + Group 13)
- Modify: `internal/filter/hcm/connection.go` (H1 dispatch site wire-in)
- Modify: `internal/filter/hcm/h2dispatch.go` (H2 dispatch site wire-in)
- Modify: `docs/envoy-go/DECISIONS.md` (author ADR-0165 IF the escape-valve fires per D12)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 4 entry + the D3-DEVIATION-FROM-SPEC §13.5 note)

Lands the callback-surface extension per planner-time decision D3 + D12. Adds **6 new methods** to `DecoderFilterCallbacks` (`DownstreamRemoteAddr` + `DownstreamLocalAddr` + `DownstreamTLSServerName` + `DownstreamTLSPeerCertDER` + `DownstreamProtocol` + `ListenerPrincipal`) per the File structure table. Mirrors the existing `tlsPrincipals` / `SetTLSPrincipals` / `(*decoderCB) DownstreamPrincipal()` pattern at chain.go:107 + 551 + 483 — 6 new chain fields + 6 new chain seeding primitives + 6 `*decoderCB` reader methods. The HCM dispatch sites (`connection.go` for H1 + `h2dispatch.go` for H2) gain 6 seeding calls alongside the existing `SetTLSPrincipals(downstreamTLSPrincipals(downstream))`. Group 13 unit tests cover seed-and-read round-trip per new method + nil/empty fall-throughs. **AMENDS SPEC §13.5 + §6.5 step 5 + §6.6 step 1-2 in-place** at the same commit — all three SPEC sections carry the "NO new callback method" claim (§13.5 the hard constraint statement; §6.5 step 5 the in-flow "NO new `DecoderFilterCallbacks` primitive"; §6.6 the "pure function ... NO `DecoderFilterCallbacks` parameter" statement) and each must be flipped for SPEC internal consistency. **Authors ADR-0165** as the cross-phase-reusable framework primitive (the ADR-0044 escape-valve firing per D12).

If at IMPL time the implementer finds the SPEC §13.5 constraint IS satisfiable (e.g., via a chain-context bag accessed without a new callback method — unlikely per D3 rationale), this task is SKIPPED + ADR-0165 does NOT land + D12 is recorded as FALSIFIED in PROGRESS.md + Tasks 5/8 are adjusted to source the state from the alternative path. The PLAN's strong hypothesis: this task FIRES.

**Precondition:** Task 3 acceptance green.
**Artifact:** 5 new callbacks; 6 new chain primitives; HCM wire-in; ADR-0165 in DECISIONS.md (CONDITIONAL).
**Acceptance:** Group 13 tests PASS; `go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...` exit 0; `go vet ./...` exit 0; `grep -nE '^## ADR-0165' docs/envoy-go/DECISIONS.md` returns 1 match (if escape-valve fired); SPEC §13.5 amended in-place at SPEC.md.

- [ ] **Step 0 (pre-spike, ≤ 5 min): grep the listener TLS plumbing** — `grep -rn '\*stdtls\.Config\|listenerTLS\|listenerCert\|tlsCert' internal/filter/hcm/connection.go internal/listener/ cmd/envoy-go/main.go` to confirm whether the listener's `*stdtls.Config.Certificates[0]` is reachable from `connection.go:dispatchRequest` via existing parameters. If reachable: Step 5 stays at ~+30 LoC. If NOT reachable: Step 5 lifts a new parameter through the dispatch chain (an additional ~30-50 LoC). Record the outcome in PROGRESS.md to tighten Task 4 LoC budget realism.

- [ ] **Step 1: Write Group 13 failing tests first** — 6 new chain seed/read round-trip tests in `chain_test.go` mirroring `TestDecoderCB_DownstreamPrincipal_SeededViaSetTLSPrincipals_ReturnsSeed` (chain_test.go:1507): one test per new field (RemoteAddr / LocalAddr / TLSServerName / TLSPeerCertDER / Protocol / ListenerPrincipal). Plus nil/empty fall-throughs.

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/ -run 'TestDecoderCB_Downstream(Remote|Local|TLSServerName|TLSPeerCertDER|Protocol)|TestDecoderCB_ListenerPrincipal' -v` — expect BUILD FAIL (the methods do not exist yet).

- [ ] **Step 3: Add 6 new methods to `DecoderFilterCallbacks`** in `callbacks.go` — doc-comments cite ADR-0165 (the impl-time-unanticipated ADR per ADR-0044 + the planner-time D3 settle) + the cross-phase reuse intent. Use `net.Addr` for the address methods (no socket-address type yet in envoy-go; `net.Addr` carries `tcp/127.0.0.1:8080` form which `addressFromNetAddr` parses at AttributeContext-build time).

- [ ] **Step 4: Add 6 new chain fields + 6 chain seeders + 6 `*decoderCB` readers** in `chain.go` — mirror the `tlsPrincipals` pattern exactly. The seeders are single-set-then-read; chain ownership invariant per ADR-0071 (set once before `RunDecodeHeaders` dispatch).

- [ ] **Step 5: Wire the H1 dispatch site** in `connection.go` at `dispatchRequest` (after the existing `chain.SetTLSPrincipals(...)` at line ~311) — 6 new seeding calls. The TLS-state seeding extracts from `*tls.Conn` if `downstream` is one (otherwise leaves the chain fields zero — `tlsServerName=""`, `peerCertDER=nil`). The `listenerPrincipal` requires reading the listener's `*stdtls.Config.Certificates[0]` — the Step-0 pre-spike informs the exact helper shape; if new parameter plumbing is needed, lift through the dispatch chain.

- [ ] **Step 6: Wire the H2 dispatch site** in `h2dispatch.go` — same 6 seeding calls. The H2 path threads chain fields via `chainDispatchAction` struct fields (mirrors `tlsPrincipals` plumbing at h2dispatch.go:45 + 145 + 203).

- [ ] **Step 7: Run Group 13 tests to verify they PASS** — `go test -race -count=1 ./internal/filter/http/... ./internal/filter/hcm/...` exit 0.

- [ ] **Step 8: Author ADR-0165 in DECISIONS.md** — `Status: Accepted`, `Date: <impl-date>`, `Lands-in: Task 4 of phase-18.2`. Body content per the ADR table at `## ADRs introduced by this plan`. Record the D3 + D12 planner-time settlement + the SPEC §13.5 / §6.5 / §6.6 deviation rationale. Insert AFTER `## ADR-0164` (the highest extant entry).

- [ ] **Step 9: AMEND SPEC §13.5 + §6.5 step 5 + §6.6 in-place** at `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` — three places carry the "NO new callback method" claim. **§13.5** replace the "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" wording with a note recording the planner-time D3 settlement + the rationale (SPEC §13.5 vs SPEC §15 item 4 conflict). **§6.5 step 5** flip the "NO new `DecoderFilterCallbacks` primitive — all this state is captured at `DecodeHeaders` time into the existing `*authRequest` struct" wording to reflect the 6-new-method callback extension. **§6.6** flip the "pure function of `*authRequest` + the four config booleans; NO `DecoderFilterCallbacks` parameter" wording to clarify that the per-stream state is captured into `*authRequest` AT DecodeHeaders time via the new callbacks; `buildAttributeContext` itself remains a pure function of `*authRequest` (the callbacks are NOT passed into `buildAttributeContext` directly). All three amendments cite ADR-0165 + the D3 settlement.

- [ ] **Step 10: Commit**

```bash
git add internal/filter/http/callbacks.go internal/filter/http/chain.go internal/filter/http/chain_test.go internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 4: callback-surface extension + 5 DecoderFilterCallbacks methods + chain seeding + Group 13 + SPEC §13.5 AMENDMENT [ADR-0165]"
```

---

## Task 5: `buildAttributeContext` in `attributes.go` + extended `*authRequest` + Group 12 [ADR-0160 gRPC-mode portion]

**Files:**
- Modify: `internal/filter/http/extauthz/attributes.go` (add `buildAttributeContext` + helpers)
- Modify: `internal/filter/http/extauthz/extauthz.go` (extend `*authRequest` struct with 10 new fields)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 12 tests)
- Modify: `docs/envoy-go/DECISIONS.md` (extend ADR-0160 with the gRPC-mode portion §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 5 entry)

Lands the gRPC-mode `AttributeContext` builder per ADR-0160 (gRPC-mode portion). Extends the `*authRequest` struct in `extauthz.go` with the 10 new fields (per D3 + Task 4): `remoteAddr`, `localAddr`, `tlsServerName`, `peerCertDER`, `listenerPrincipal`, `protocol`, `requestID`, `streamStartTime`, `perRouteContextExtensions`, `downstreamPrincipal`. Authors `buildAttributeContext` in `attributes.go` per SPEC §6.6 — pure function of `*authRequest` + the four boolean gates. Steps 1–10 per the File structure table responsibility. Helpers: `addressFromNetAddr`, `lowercaseHeaderMap`, `firstOrEmpty`, `bodyStringIfNotBytes`/`bodyBytesIfBytes`. Group 12 covers populated-set per §11.P4 evidence: pseudo-headers lowercased + included; `request.time` non-zero; `source/destination.address.socket_address` from `req.remoteAddr/localAddr`; `source.principal` from first of `req.downstreamPrincipal`; `destination.principal` from `req.listenerPrincipal` (AUTOMATICALLY populated, NOT gated); `tls_session.sni` gated by `includeTlsSession && req.tlsServerName != ""`; `source.certificate` gated by `includePeerCert && len(req.peerCertDER) > 0`; `metadata_context` + `route_metadata_context` populated as empty messages (NOT nil); `context_extensions` from `req.perRouteContextExtensions`; `pack_as_bytes` honored; `encodeRawHeaders` DEFERRED per D6 (`header_map` stays empty; legacy `headers` populated).

**Precondition:** Task 4 acceptance green.
**Artifact:** `attributes.go` extended with `buildAttributeContext` + helpers; `*authRequest` extended; ADR-0160 gRPC-mode portion in DECISIONS.md.
**Acceptance:** Group 12 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet ./...` exit 0; `grep -nE '^## ADR-0160' docs/envoy-go/DECISIONS.md` returns 1 match (with the gRPC-mode portion landed).

- [ ] **Step 1: Write Group 12 failing tests first** — populated-set per §11.P4 evidence; gate conditional populations (`tls_session.sni` / `source.certificate`); empty-message vs nil for `metadata_context`/`route_metadata_context`; pseudo-header inclusion + lowercasing; `pack_as_bytes` honored; `encode_raw_headers` DEFERRED (no `header_map` populated).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestBuildAttributeContext' -v`.

- [ ] **Step 3: Extend `*authRequest`** in `extauthz.go` — add 10 new fields per D3. Update existing 18.1 tests that construct `*authRequest` directly (in extauthz_test.go) to set sensible defaults for the new fields (zero values mostly fine).

- [ ] **Step 4: Author `buildAttributeContext`** in `attributes.go` — steps 1–10 per the File structure table + SPEC §6.6. Author helpers: `addressFromNetAddr`, `lowercaseHeaderMap`, `firstOrEmpty`, `bodyStringIfNotBytes`/`bodyBytesIfBytes`. The §11.P4 in-session evidence is the ground truth.

- [ ] **Step 5: Run tests to verify they PASS** — Group 12 + re-run all prior groups (1–9 from 18.1 + 10 + 11 + 13 if they exist already — only 13 from Task 4 exists at this point; 10/11 land at later tasks; the existing 18.1 groups should still PASS).

- [ ] **Step 6: Extend ADR-0160 in DECISIONS.md** — IN-PLACE extension. The existing 18.1 §Decision + §Consequences body covers the HTTP-mode portion (the `AuthorizationRequest` builder). Add a `### gRPC-mode portion (lands at phase-18.2 Task 5)` sub-heading + the gRPC-mode body per the ADR table. `Date: <impl-date>` updated.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 5: attributes.go buildAttributeContext + extended *authRequest + Group 12 [ADR-0160 gRPC-mode portion]"
```

---

## Task 6: `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` in `check.go` + Group 11 [ADR-0161 gRPC-mode portion]

**Files:**
- Modify: `internal/filter/http/extauthz/check.go` (real `buildGRPCCheckFn` body + `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC`)
- Modify: `internal/filter/http/extauthz/extauthz.go` (extend `checkDisposition` with `upstreamDel []string` field per D5)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 11 tests; update existing Group 1 tests whose `grpc_service` PARSE-REJECT error wording changed at Task 3)
- Modify: `docs/envoy-go/DECISIONS.md` (extend ADR-0161 with the gRPC-mode portion §Decision + §Consequences)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 6 entry)

Lands the gRPC-mode `CheckResponse` → `checkDisposition` mapping per ADR-0161 (gRPC-mode portion). Fills the Task-3-stubbed `buildGRPCCheckFn` body: closure validates `gs.TargetSpecifier` (PARSE-REJECT on `*GoogleGrpc_`), looks up the cluster (PARSE-REJECT on unknown / `UseH2()==false`), constructs `*grpcclient.AuthClient`, returns the closure body `(ctx, req) → {attrCtx := buildAttributeContext(...); checkReq := &authv3.CheckRequest{Attributes: attrCtx}; resp, err := ac.Check(ctx, checkReq); if err != nil return dispError, err; return mapGRPCResponse(resp, validateMutations), nil}`. `mapGRPCResponse` dispatches on `resp.HttpResponse` oneof + `resp.GetStatus().GetCode()` per SPEC §6.7. `buildAllowDispositionGRPC` extracts `OkHttpResponse.headers` per the 4-arm `append_action` dispatch table per D5; extracts `headers_to_remove` into the NEW `upstreamDel []string` field on `checkDisposition`; runs `validateMutationHeaders` if gated. `buildDenyDispositionGRPC` extracts `DeniedHttpResponse.{status.code, body, headers}` verbatim (NO `allowed_client_headers` filter — UNLIKE HTTP-mode). `checkDisposition` is extended with `upstreamDel []string`; `applyUpstreamMutations` (extauthz.go:676) is extended to honor `upstreamDel` (delete from upstream headers); `headerKV` is extended with the per-D5 dispatch discriminators (planner-time decision D5 settles the exact representation).

**Precondition:** Task 5 acceptance green.
**Artifact:** `check.go` extended with the 4 new functions; `checkDisposition` + `headerKV` extended; ADR-0161 gRPC-mode portion in DECISIONS.md.
**Acceptance:** Group 11 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet ./...` exit 0; `grep -nE '^## ADR-0161' docs/envoy-go/DECISIONS.md` returns 1 match with gRPC-mode portion landed; ALL existing 18.1 Groups 1–9 + Tasks 4/5's Groups 12+13 STILL PASS.

- [ ] **Step 1: Write Group 11 failing tests first** — `mapGRPCResponse` truth table per SPEC §6.7: OK+OkResponse → allow; OK+OkResponse+mutations → upstream set/append/del populated per all 4 `append_action` arms; OK+DeniedResponse → dispError; non-OK+DeniedResponse → deny verbatim headers/body/status; non-OK+OkResponse → dispError; non-OK+nil-oneof → deny default 403; OK+nil-oneof → allow; `validate_mutations:true` rejection → dispInvalid + invalid counter; `response_headers_to_add` silent-ignored per D11; transport error → dispError (already covered by Task 3's grpcclient tests; here we cover the filter's mapping path).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestMapGRPCResponse|TestBuildAllowDispositionGRPC|TestBuildDenyDispositionGRPC' -v`.

- [ ] **Step 3: Extend `checkDisposition` + `headerKV`** in `extauthz.go` — add `upstreamDel []string` field on `checkDisposition`; add the D5 dispatch discriminators on `headerKV` (the IMPL settles the exact representation — a 4-value enum field is cleaner than two booleans per D5). Update `applyUpstreamMutations` (extauthz.go:676) to honor `upstreamDel` (`headers.Del(name)` for each entry) + to handle the D5 dispatch discriminators correctly (the SET-IF-PRESENT and ADD-IF-ABSENT cases per D5).

- [ ] **Step 4: Implement `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC`** in `check.go` per SPEC §6.7. Reuse `validateMutationHeaders` from `attributes.go` (the 18.1 routine; mode-agnostic).

- [ ] **Step 5: Replace `buildGRPCCheckFn` stub with the real body** in `check.go` — the Task-3 stub returns `nil, errors.New("ext_authz: grpc_service: TODO (Task 5)")`; replace with the full body per SPEC §6.5 (cluster lookup + `UseH2()` gate + `GoogleGrpc` PARSE-REJECT + `*grpcclient.AuthClient` construction + closure return).

- [ ] **Step 6: Update existing 18.1 Group 1 tests** that PARSE-REJECT on `grpc_service` configs — the error wording changed at Task 3 (no longer `"grpc_service mode not yet supported (lands in phase 18.2)"`; now `"ext_authz: grpc_service: unknown cluster <name>"` or similar from `buildGRPCCheckFn`). The Group 1 tests are updated to assert the NEW PARSE-REJECT error wording for the now-activated arm.

- [ ] **Step 7: Run tests to verify they PASS** — Group 11 + re-run all prior groups; the previously-failing Group 1 tests now pass with the new error wording.

- [ ] **Step 8: Extend ADR-0161 in DECISIONS.md** — IN-PLACE extension. Add a `### gRPC-mode portion (lands at phase-18.2 Task 6)` sub-heading + the gRPC-mode body per the ADR table.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/DECISIONS.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 6: check.go mapGRPCResponse + buildAllowDispositionGRPC + buildDenyDispositionGRPC + Group 11 [ADR-0161 gRPC-mode portion]"
```

---

## Task 7: `context_extensions` consumption + per-route override threading + Group 14 (mode-agnostic per-route in gRPC mode)

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (extend `compiledCheckSettings` with the threading discipline; extend `dispatchOutboundCheck` to seed `req.perRouteContextExtensions` from the resolved `*compiledPerRoute`)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 14 — per-route `context_extensions` flows into `AttributeContext.context_extensions` in gRPC mode)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 7 entry)

Activates the `CheckSettings.context_extensions` field that 18.1 parsed-but-NO-OPed per the proto doc-note (gRPC-mode-only). At 18.1, the field was parsed in `parsePerRoute` (line extauthz.go:537 — `cs.GetContextExtensions()` stored on `compiledCheckSettings.contextExtensions`); 18.2 threads it into `AttributeContext.context_extensions` via the extended `*authRequest.perRouteContextExtensions`. **NO listener-level baseline at MVP** — `ExtAuthz` has no top-level `context_extensions` field, and `core.GrpcService.initial_metadata` is DEFERRED per SPEC §2.6 + §8 item 2; the EFFECTIVE map is the per-route `CheckSettings.context_extensions` map OR empty. Per-route wins on key collisions (proto map-merge convention per SPEC §5). Group 14 unit tests cover the threading + the empty-fallback. **NO new ADR** — the field is already covered by ADR-0163's 5th-canonical-REUSE discipline; the 18.1 SPEC §8 forward-pointer (item 8) is CLOSED by this task (recorded in PROGRESS.md + BEHAVIOR_CONTRACT §13.4 at Task 13).

**Precondition:** Task 6 acceptance green.
**Artifact:** `extauthz.go` extended with `req.perRouteContextExtensions` seeding from per-route resolved `*compiledPerRoute`; Group 14 tests.
**Acceptance:** Group 14 tests PASS; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0; `go vet ./...` exit 0.

- [ ] **Step 1: Write Group 14 failing tests first** — per-route `check_settings.context_extensions: {policy: "value"}` flows into `AttributeContext.context_extensions[policy] == "value"`; empty/nil per-route → empty map; key collision merge semantics (none at MVP since no listener-level source).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestContextExtensionsThreading' -v`.

- [ ] **Step 3: Wire `req.perRouteContextExtensions = perRouteContextExtensionsFor(f.perRoute)`** in `dispatchOutboundCheck` — the helper returns the per-route `compiledCheckSettings.contextExtensions` map (mod nil-handling) when the per-route is `*compiledCheckSettings`-arm; empty map otherwise.

- [ ] **Step 4: Run tests to verify they PASS** — Group 14 + re-run all prior groups.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 7: context_extensions consumption + per-route threading + Group 14"
```

---

## Task 8: `*authRequest` per-stream seeding in `dispatchOutboundCheck` + Group 10 (parse-time gRPC arm validation) + race-test exercise

**Files:**
- Modify: `internal/filter/http/extauthz/extauthz.go` (`dispatchOutboundCheck` seeds 10 extended `*authRequest` fields from new callbacks + `headers` + `f.streamStartTime`)
- Modify: `internal/filter/http/extauthz/extauthz_test.go` (append Group 10 — parse-time gRPC arm validation)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 8 entry)

Wires the extended `*authRequest` field seeding in `dispatchOutboundCheck` per SPEC §6.5 + the Task 4 callback extension. Group 10 covers parse-time validation of the `grpc_service` arm: unknown cluster name → PARSE-REJECT (`"ext_authz: grpc_service: unknown cluster X"`); `UseH2()==false` cluster → PARSE-REJECT (`"ext_authz: grpc_service: cluster X must have http2_protocol_options{} set"`); `GoogleGrpc` arm → PARSE-REJECT envoy-go-strict; `EnvoyGrpc.cluster_name` empty → PARSE-REJECT; happy path returns a non-nil `checkFn`. Race-test exercise: `OnDestroy` cancels the in-flight `(*AuthClient).Check` via `context.Canceled` propagation; concurrent `Check` calls on the same `*AuthClient` from multiple per-stream goroutines (gRPC `ClientConn` is goroutine-safe per upstream docs); the existing `mu`/`done` guard from 18.1 protects the resume-after-`OnDestroy` race for gRPC mode identically to HTTP mode.

**Precondition:** Task 7 acceptance green.
**Artifact:** `extauthz.go` extended seeding; Group 10 tests.
**Acceptance:** Group 10 tests PASS; race-tests PASS for the gRPC mode under `-race`; `go test -race -count=1 ./internal/filter/http/extauthz/...` exit 0.

- [ ] **Step 1: Write Group 10 failing tests first** — parse-time gRPC arm validation per the File structure table responsibility + `TestOnDestroy_CancelsInFlightGRPCCheck` (parallel to 18.1's HTTP-mode equivalent).

- [ ] **Step 2: Run tests to verify they FAIL** — `go test ./internal/filter/http/extauthz/ -run 'TestBuildGRPCCheckFn|TestOnDestroy_CancelsInFlightGRPCCheck' -v`.

- [ ] **Step 3: Wire `*authRequest` extended-field seeding** in `dispatchOutboundCheck` per the File structure table for `extauthz.go`. Capture `f.streamStartTime = time.Now()` at the START of DecodeHeaders (or alternatively, seed at `dispatchOutboundCheck` time — the IMPL settles; the §11.P4 evidence shows `request.time` is the request-arrival time, not the auth-call time, so DecodeHeaders entry is the right anchor).

- [ ] **Step 4: Run tests to verify they PASS** — Group 10 + race-test + re-run all prior groups.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/extauthz/ docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 8: *authRequest extended-field seeding + Group 10 + race-test exercise"
```

---

## Task 9: 23rd fuzzer `FuzzCheckResponseMapping` + corpus extension to `FuzzExtAuthzConfigParse` + NEW `test/helpers/extauthzgrpc/` test-helper + fixture infrastructure (BackendKind enum)

**Files:**
- Modify: `internal/filter/http/extauthz/fuzz_test.go` (add `FuzzCheckResponseMapping` + extend `FuzzExtAuthzConfigParse` corpus with `grpc_service` variants)
- Create: `test/helpers/extauthzgrpc/doc.go`
- Create: `test/helpers/extauthzgrpc/extauthzgrpc.go`
- Create: `test/helpers/extauthzgrpc/extauthzgrpc_test.go`
- Modify: `test/differential/fixture/fixture.go` (`HTTPExtAuthzGRPC BackendKind = 18`)
- Modify: `test/differential/runner_test.go` (blank import + switch-case)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 9 entry)

Lands the 23rd fuzzer + the NEW shared in-process gRPC `Authorization/Check` test-helper + the fixture infrastructure. `FuzzCheckResponseMapping` fuzzes arbitrary bytes as `*authv3.CheckResponse` proto-bytes → `proto.Unmarshal` → `mapGRPCResponse` per SPEC §7.3. The existing `FuzzExtAuthzConfigParse` corpus extends with `grpc_service` config variants (config-parse path automatically covered without a new fuzzer). `test/helpers/extauthzgrpc/` is the NEW FIRST in-process gRPC server (planner-time decision D1; mirrors `test/helpers/extauthzhttp/`). `test/differential/fixture/fixture.go` gains the `HTTPExtAuthzGRPC` `BackendKind`; `runner_test.go` gains the blank import + the dispatch switch-case.

**Precondition:** Task 8 acceptance green.
**Artifact:** 23rd fuzzer + extended 22nd fuzzer corpus + `test/helpers/extauthzgrpc/` + fixture infrastructure.
**Acceptance:** `go build ./...` exit 0; `go vet ./...` exit 0; `go test -race -count=1 ./test/helpers/extauthzgrpc/...` exit 0; `FuzzCheckResponseMapping` runs clean for 30s (`go test -run '^$' -fuzz 'FuzzCheckResponseMapping' -fuzztime 30s ./internal/filter/http/extauthz/`); `FuzzExtAuthzConfigParse` re-runs clean for 30s with the extended corpus.

- [ ] **Step 1: Write the `test/helpers/extauthzgrpc/extauthzgrpc_test.go` failing tests first** — `TestNew_StartsServerOnEphemeralPort`; `TestServer_Script_ReturnsScripted`; `TestServer_Stop_Closes`; `TestServer_ConcurrentClient_NoRace`.

- [ ] **Step 2: Author `test/helpers/extauthzgrpc/{doc.go, extauthzgrpc.go}`** — per the File structure table responsibility; mirror `test/helpers/jwksbackend/` + `test/helpers/extauthzhttp/` structure. Per planner-time decision D1: `:path`-keyed Script.

- [ ] **Step 3: Run the test-helper tests to verify they PASS** — `go test -race ./test/helpers/extauthzgrpc/...`.

- [ ] **Step 4: Author `FuzzCheckResponseMapping`** in fuzz_test.go per SPEC §7.3 + the File structure table responsibility. Seed the corpus with 6–10 variants covering all `mapGRPCResponse` truth-table cases.

- [ ] **Step 5: Extend `FuzzExtAuthzConfigParse` corpus** with `grpc_service` config variants — `EnvoyGrpc.cluster_name` valid/empty/unknown; `GoogleGrpc` arm; `initial_metadata` populated; `retry_policy` populated; `transport_api_version` non-V3.

- [ ] **Step 6: Wire the fixture infrastructure** — `HTTPExtAuthzGRPC BackendKind = 18` in `fixture.go`; the blank import + dispatch switch-case in `runner_test.go`.

- [ ] **Step 7: Verify** — `go build ./...` + `go vet ./...` + the two 30s fuzz runs + `go test -race ./test/helpers/extauthzgrpc/...`.

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/extauthz/fuzz_test.go test/helpers/extauthzgrpc/ test/differential/ docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 9: FuzzCheckResponseMapping 23rd fuzzer + corpus extension + test/helpers/extauthzgrpc/ + fixture infrastructure"
```

---

## Task 10: Fixture `0021-http-ext-authz-grpc` — `inputs/driver.go` (8-scenario driver + extauthzgrpc lifecycle + counter-delta scrape)

**Files:**
- Create: `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go`
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 10 entry)

Lands the 8-scenario differential driver per SPEC §7.1. Functions `runScenario1..runScenario8(ctx, baseURLs, authBaseURL) error` where `baseURLs` is a map of listener name → URL (l_test_a/b/c per D10). Per-scenario assertion: byte-exact body + status equivalence + `/stats/prometheus` counter-delta on the 5 reachable counters + backend-arrival header assertions + auth-server received-CheckRequest content assertions (scenario 7 — `context_extensions[policy] == "scenario7"`). Includes the `setupAuthGRPC` lifecycle helper (scenarios 3+4 stop it before the request) + the `scrapeStats`/`assertCounterDelta` helpers. Pre-populates the 8 scripted `CheckResponse` values via `srv.Script(":path-discriminator", resp)` before issuing requests.

**Precondition:** Task 9 acceptance green.
**Artifact:** `test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go`.
**Acceptance:** `go build ./test/fixtures/0021-http-ext-authz-grpc/...` exit 0; `go vet` exit 0 (the driver compiles; the end-to-end differential run is Task 12).

- [ ] **Step 1: Author `inputs/driver.go`** — the 8-scenario driver + the extauthzgrpc lifecycle helper + the counter-delta helpers, per the File structure table responsibility + the SPEC §7.1 per-request matrix.

- [ ] **Step 2: Verify it compiles** — `go build ./test/fixtures/0021-http-ext-authz-grpc/... && go vet ./test/fixtures/0021-http-ext-authz-grpc/...`.

- [ ] **Step 3: Commit**

```bash
git add test/fixtures/0021-http-ext-authz-grpc/inputs/ docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 10: fixture 0021 driver.go — 8-scenario differential driver"
```

---

## Task 11: Fixture `0021-http-ext-authz-grpc` — `envoy.yaml` + `envoy-go.yaml` bootstraps (three-listener topology)

**Files:**
- Create: `test/fixtures/0021-http-ext-authz-grpc/envoy.yaml`
- Create: `test/fixtures/0021-http-ext-authz-grpc/envoy-go.yaml`
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 11 entry)

Lands the two bootstrap configs per SPEC §7.2 + planner-time decision D10. Both wire 3 HCM listeners `l_test_a/b/c` (plaintext TCP; HCM chain `ext_authz → router`) with per-listener `ExtAuthz` config (gRPC-mode: `grpc_service.envoy_grpc.cluster_name: c_authz_grpc`; `transport_api_version: V3`; `with_request_body` for scenario 5; per-listener `failure_mode_allow` / `status_on_error` as required by D10). Routes per the 8 scenarios. Cluster `c_authz_grpc` with mandatory `http2_protocol_options: {}` (gRPC framing requirement per §11.P13). `envoy.yaml` uses STRICT_DNS; `envoy-go.yaml` uses STATIC.

**Precondition:** Task 10 acceptance green.
**Artifact:** `envoy.yaml` + `envoy-go.yaml`.
**Acceptance:** both YAML files parse (envoy-go config-validation entry-point + the v1.37.2 image config-check).

- [ ] **Step 1: Author `envoy.yaml`** — the reference Envoy bootstrap per the File structure table responsibility.

- [ ] **Step 2: Author `envoy-go.yaml`** — the equivalent envoy-go bootstrap (STATIC clusters).

- [ ] **Step 3: Validate both configs** — envoy-go config-validation entry-point on `envoy-go.yaml`; the v1.37.2 image config-check on `envoy.yaml`.

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0021-http-ext-authz-grpc/envoy.yaml test/fixtures/0021-http-ext-authz-grpc/envoy-go.yaml docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 11: fixture 0021 envoy.yaml + envoy-go.yaml bootstraps (three-listener topology)"
```

---

## Task 12: Fixture `0021-http-ext-authz-grpc` — `expectations.yaml` + `README.md` + end-to-end differential pass (all 8 scenarios + all 22 fixtures)

**Files:**
- Create: `test/fixtures/0021-http-ext-authz-grpc/expectations.yaml`
- Create: `test/fixtures/0021-http-ext-authz-grpc/README.md`
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 12 entry)

Lands `expectations.yaml` + `README.md` and runs the end-to-end differential to green. No RATIFIED-PENDING-IMPL-TIME pin closure at this task (all 13 parent §5 pins closed at the 18.2 SPEC commit per SPEC §11). If the differential diff DIVERGES on byte-exactness, iterate on driver/config/filter code until all 8 scenarios + all 22 fixtures pass.

**Precondition:** Task 11 acceptance green.
**Artifact:** `expectations.yaml` + `README.md`; the differential suite green.
**Acceptance:** `go test -count=1 ./test/differential/ -run 'Test.*0021'` PASS (all 8 scenarios); `go test -count=1 ./test/differential/` PASS (all 22 fixtures 0000–0021).

- [ ] **Step 1: Author `expectations.yaml`** — per-scenario allow-list + counter-delta map per the File structure table responsibility.

- [ ] **Step 2: Author `README.md`** — fixture overview + 8-scenario list + three-listener topology rationale + divergence-window note.

- [ ] **Step 3: Run the fixture 0021 differential** — `go test -count=1 ./test/differential/ -run 'Test.*0021'`; iterate on the driver / configs / filter code until all 8 scenarios PASS.

- [ ] **Step 4: Run the full differential suite** — `go test -count=1 ./test/differential/` — all 22 fixtures (0000–0021) PASS.

- [ ] **Step 5: Commit**

```bash
git add test/fixtures/0021-http-ext-authz-grpc/expectations.yaml test/fixtures/0021-http-ext-authz-grpc/README.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 12: fixture 0021 expectations + README + end-to-end differential pass"
```

---

## Task 13: BEHAVIOR_CONTRACT.md 8-edit bundle + ROADMAP rows 18.2 + 18 in-progress→done (SAME COMMIT) + STATE.md advance + 6-gate phase-done verification

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (§13.1 + §13.2 + §13.3 + §13.4 + §13.5 + §13.7 — 8 patches per SPEC §13)
- Modify: `docs/envoy-go/ROADMAP.md` (rows 18.2 AND 18 both flip `in-progress → done` AT THE SAME COMMIT)
- Modify: `docs/envoy-go/STATE.md` (advance per BOOTSTRAP_PROMPT.md §5)
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 13 entry + the 6-gate report)

Lands the BEHAVIOR_CONTRACT.md 8-patch bundle per SPEC §13, flips ROADMAP rows 18.2 AND 18 (BOTH at the SAME commit per parent SPEC §8 rollup), advances STATE.md, and runs the 6 phase-done gates per BOOTSTRAP_PROMPT.md §7.5. **CRITICAL: the commit-message body MUST explicitly name BOTH `row 18.2 in-progress → done` AND `row 18 in-progress → done` for grep-verifiability per SPEC §15 item 13.**

**Precondition:** Task 12 acceptance green.
**Artifact:** BEHAVIOR_CONTRACT.md + ROADMAP.md + STATE.md updated; the 6-gate report appended to PROGRESS.md.
**Acceptance:** All 6 phase-done gates green per SPEC §14.5:
- **Gate A** (build + vet + lint): `go build ./...` exit 0; `go vet ./...` exit 0; `golangci-lint run` exit 0.
- **Gate B** (race tests): `go test -race -count=1 ./...` exit 0 repo-wide including the new `grpcclient` + `extauthzgrpc` packages.
- **Gate C** (h2spec): 53/53 PASS at the ADR-0051 pin (no H2 wire-shape change; ext_authz uses gRPC over H2 to the upstream auth cluster, not to the downstream client per SPEC §14.4).
- **Gate D** (fuzzers): 23 fuzzers green at 30s each.
- **Gate E** (differential): 22 differential fixtures (0000–0021) PASS.
- **Gate F** (BEHAVIOR_CONTRACT): the §13 8-patch bundle landed.

- [ ] **Step 1: Apply BEHAVIOR_CONTRACT.md §13.1 patch** — flip the "gRPC mode — see phase 18.2" forward-pointers in the `### envoy.filters.http.ext_authz` subsection to substantive gRPC content per the File structure table for BEHAVIOR_CONTRACT.md.

- [ ] **Step 2: Apply §13.2 patch** — stat-table 77 names UNCHANGED + add the clarification headnote that 0 new counters land at 18.2.

- [ ] **Step 3: Apply §13.3 patch** — Equivalence-Matrix new row for fixture `0021-http-ext-authz-grpc`.

- [ ] **Step 4: Apply §13.4 patch** — NEW `### Phase 18.2 forward-pointer notes` subsection covering the SPEC §8 12-item deferral list + the 2 closures from 18.1 (gRPC arm activation + `context_extensions` consumption).

- [ ] **Step 5: Apply §13.5 patch** — AMEND in-place: flip the 18.1-anchored "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" to reflect the 5 new callback methods landed at Task 4 per D3 + ADR-0165. Record the PLAN-time deviation rationale (SPEC §13.5 vs SPEC §15 item 4 conflict).

- [ ] **Step 6: Apply §13.7 patch** — NEW top-level `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella per the File structure table.

- [ ] **Step 7: Apply §13.8** — existing `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)` UNCHANGED (cross-reference verification only).

- [ ] **Step 8: Flip ROADMAP row 18.2 + row 18 BOTH to `in-progress → done`** AT THE SAME COMMIT. Update row 18.2 summary with post-impl counts (14-task + final ADR roster + the ADR-0044 escape-valve disposition); update row 18 summary to reflect parent-rollup closure (both sub-phases now `done`).

- [ ] **Step 9: Advance STATE.md** — per BOOTSTRAP_PROMPT.md §5 lifecycle. `active-phase: <next-phase>` (the next §9 family-row — see BOOTSTRAP §9 family list); `lifecycle-state: phase 18.2 done; phase 18 done; phase <next> BRAINSTORM pending` (or analog); `next-skill: superpowers:brainstorming` (for the next phase's BRAINSTORM); `last-commit: <Task 13 squash>`; `next-free ADR: ADR-0166` (or `ADR-0165` if D12 was falsified at Task 4); `last-updated: <impl-date>`.

- [ ] **Step 10: Run the 6-gate phase-done verification** — execute all 6 gate commands; capture verbatim outputs into the PROGRESS.md Task 13 entry; all green.

- [ ] **Step 11: Commit**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/ROADMAP.md docs/envoy-go/STATE.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "$(cat <<'EOF'
phase 18.2 Task 13: BEHAVIOR_CONTRACT 8-edit bundle + ROADMAP row 18.2 in-progress→done + row 18 in-progress→done + STATE advance + 6-gate phase-done verification

Closes BOTH row 18.2 AND parent row 18 at the same commit per parent SPEC §8
parent-rollup discipline. Phase-18 ADR-0045 split now closed:
  - row 18.1 done (2026-05-15; 7 ADRs landed)
  - row 18.2 done (this commit; 4–5 ADRs landed)
  - row 18 done (this commit; parent-rollup per phase-18 SPEC §8)
EOF
)"
```

---

## Task 14: REVIEW.md — end-of-phase review per `superpowers:requesting-code-review` skill

**Files:**
- Create: `docs/envoy-go/phases/18.2-ext-authz-grpc/REVIEW.md`
- Modify: `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (append Task 14 entry)

Lands the end-of-phase review document per `superpowers:requesting-code-review`. Covers: the 18.2 deliverables + the final ADR roster (4–5 anchored — ADR-0158 + ADR-0157 §Decision AMENDMENT + ADR-0160 gRPC-mode + ADR-0161 gRPC-mode; + ADR-0165 if D12 hypothesis held); the SPEC §15 15-claim acceptance checklist verification; the 18.2-load-bearing §11 empirical-pin dispositions (the §11.P4 + §11.P13 in-session RATIFICATIONS at SPEC time — closed before IMPL); the framework-delta impact (ONE new primitive — ADR-0158 the gRPC-client outbound framework primitive at `internal/grpcclient/` — + FIVE REUSES per SPEC §3: `internal/cluster.Manager` + ADR-0144 `DownstreamPrincipal()` + phase-09 async-resume + phase-13 ADR-0128 body-buffering + ADR-0085 `SendLocalReply`; PLUS the callback-surface extension per ADR-0165 if it fired); the divergence-window enumeration (`OkHttpResponse.response_headers_to_add` DEFERRED; `header_map` arm DEFERRED per D6; envoy-go-strict treatment of `OkResponse+non-zero-status` AND `DeniedResponse+zero-status` as `dispError` per SPEC §6.7; `core.GrpcService.{initial_metadata, retry_policy, google_grpc}` SILENT-IGNORED / PARSE-REJECT); the parent-rollup note (parent row 18 closed AT THE SAME COMMIT as row 18.2 per Task 13's commit message); the SPEC §13.5 PLAN-time AMENDMENT (the callback-surface extension was unavoidable per the SPEC §15 item 4 conflict — recorded as a planner-time deviation + landed at Task 4); the cross-phase reuse anticipation (ext_proc + global_ratelimit will reuse `internal/grpcclient/Dialer` + the callback-surface extension per ADR-0158 §Consequences + ADR-0165 §Consequences).

**Precondition:** Task 13 acceptance green.
**Artifact:** new REVIEW.md file.
**Acceptance:** REVIEW.md committed; the 18.2 end-state captured.

- [ ] **Step 1: Author REVIEW.md** — structure per the `superpowers:requesting-code-review` skill output template + the phase-13..18.1 REVIEW.md precedent. ~240 LoC.

- [ ] **Step 2: Commit**

```bash
git add docs/envoy-go/phases/18.2-ext-authz-grpc/REVIEW.md docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md
git commit -m "phase 18.2 Task 14: REVIEW.md — end-of-phase review"
```

---

## End of phase 18.2 implementation plan
