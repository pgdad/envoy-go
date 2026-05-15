# Phase 18.2 — Code review (REVIEW.md)

**Phase id:** `18.2` (TWELFTH §9 HTTP-filters family-row to land per ADR-0106; the SECOND half of the ADR-0045 phase-18 split per ADR-0164 — sibling to 18.1; FIRST §9 row to introduce a brand-new outbound-RPC framework primitive — `internal/grpcclient/` via ADR-0158; FIRST §9 row to fire the ADR-0044 escape-valve TWICE in one sub-phase — ADR-0165 at PLAN-time per planner-decision D12, ADR-0166 at IMPL-fixup time per Task 11.5; closes the parent phase-18 ADR-0045 split — row 18 + row 18.2 BOTH flip `done` AT THE SAME COMMIT per parent SPEC §8)
**Slug:** `18.2-ext-authz-grpc`
**Branch under review:** `phase-18.2-ext-authz-grpc-impl`
**Range:** branch tip `620548ec` (Task 13 BEHAVIOR_CONTRACT 8-edit + ROADMAP 18.2+18 done + STATE advance + 6-gate verification — all GREEN at this SHA; REVIEW.md is the final Task 14 commit). 14 task-landing commits (Tasks 1–13 + the Task 11.5 fixup) + multiple in-task follow-up commits at worktree HEAD. The last-commit SHA-fill is deferred to the post-`wt-merge` follow-up per the phase-09..18.1 IMPL-stage close pattern.
**Parent ROADMAP row:** `18.2 ext-authz-grpc` flipped `in-progress → done` at Task 13 commit `620548ec` (date `2026-05-15`). **Parent row `18 http-filter-ext-authz` ALSO flipped `in-progress → done` at the SAME commit** per parent SPEC §8 parent-rollup discipline — the phase-18 ADR-0045 split is now fully closed (18.1 done at `3cc8182` + 18.2 done at `620548ec` + parent 18 done at `620548ec`). The commit-message body for `620548ec` explicitly names BOTH transitions for grep-verifiability.
**Reviewer method:** Inline authoring by the implementing session — inputs: SPEC §15 15-claim acceptance checklist + PLAN's 14-task structure + the branch diff (`+10226 / -149 LoC` across 50 files; production-code subset `+7932 / -106` across 25 files) + PROGRESS.md per-task entries (Tasks 1–13 + Task 11.5 fixup with verbatim outputs) + DECISIONS.md ADR-0157 §Decision AMENDMENT + ADR-0158 full body + ADR-0160 gRPC-mode portion + ADR-0161 gRPC-mode portion + ADR-0165 + ADR-0166 + BEHAVIOR_CONTRACT §13 8-edit bundle + phase-18.1 + phase-17 + phase-13 REVIEW.md structural template precedents.
**Six-gate state at HEAD:** all GREEN per Task 13's verification sweep — outputs reproduced verbatim in §11 below.

This review covers the full phase-18.2 surface: the NEW `internal/grpcclient/` framework primitive (`doc.go` 86 + `grpcclient.go` 241 = **327 LoC production**; `grpcclient_test.go` 820 LoC); the gRPC-mode extensions to `internal/filter/http/extauthz/` (`extauthz.go` +311 / `attributes.go` +275 / `check.go` +383 production deltas; `extauthz_test.go` +2195 / `fuzz_test.go` +412); the NEW callback-surface extension per ADR-0165 (`callbacks.go` +74 / `chain.go` +119 / `chain_test.go` +246 / `callbacks_test.go` +11 / sister-package test-mock extensions across 10 conforming types); the HCM dispatch wire-in (`hcm/connection.go` +28 / `hcm/h2dispatch.go` +73 / `hcm/config.go` +29 / `hcm/filter.go` +21); the listener-principal plumbing (`listener/manager.go` +75); the Task 11.5 cluster-manager plaintext h2c upstream relaxation per ADR-0166 (`cluster/manager.go` ~+30 / `cluster/dial_h2.go` ~+80 + test deltas); the NEW in-process gRPC `Authorization/Check` test-helper at `test/helpers/extauthzgrpc/` (`extauthzgrpc.go` 179 + `doc.go` 31 + `extauthzgrpc_test.go` 290 = the FIRST in-process gRPC server in envoy-go's test tree); the NEW differential fixture `0021-http-ext-authz-grpc` (8 scenarios; 3-listener topology; `envoy.yaml` 244 + `envoy-go.yaml` 218 + `expectations.yaml` 314 + `README.md` 259 + `inputs/driver.go` 1172); fixture-runner wiring (`test/differential/fixture/fixture.go` +17 + `test/differential/runner_test.go` +34); the 23rd fuzzer `FuzzCheckResponseMapping` + `FuzzExtAuthzConfigParse` `grpc_service` corpus extension; the BEHAVIOR_CONTRACT.md 8-edit bundle; the 6 ADRs touched (ADR-0157 §Decision AMENDMENT, ADR-0158 full, ADR-0160 gRPC-mode portion, ADR-0161 gRPC-mode portion, ADR-0165 NEW, ADR-0166 NEW); the ROADMAP row 18.2 + parent row 18 BOTH `done`; the STATE.md advance to lifecycle-state-4.

This REVIEW closes phase 18.2's lifecycle (state-4) AND closes the parent phase-18 row at the same commit, completing the ADR-0045 split. It is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.** All six phase-done gates are GREEN at HEAD `620548ec` per Task 13's verification sweep. The implementation faithfully realizes the SPEC across all 14 PLAN tasks (Tasks 1–13 + Task 11.5 IMPL-time fixup). ext_authz gRPC service mode is the TWELFTH §9 HTTP-filters family-row to ship under ADR-0106; it closes the parent phase-18 ADR-0045 split (`18-http-filter-ext-authz`).

The architectural centerpiece is the **NEW `internal/grpcclient/` framework primitive (ADR-0158)** — envoy-go's FIRST gRPC infrastructure of any kind. A thin generic `Dialer` constructs `*grpc.ClientConn` values for envoy-go cluster references by coupling to `internal/cluster.Manager` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` — TLS terminates at the cluster-manager layer per the §11.P13 in-session SPEC scrape. A typed `AuthClient` wrapper composes the `envoy.service.auth.v3.AuthorizationClient` stub (ships in go-control-plane v1.32.4 — no codegen) on top of the `Dialer`. One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair, created at config-load time + shared across per-stream Check calls + leaks-on-exit MVP per planner-decision D2.

The `compiledConfig` shape is UNCHANGED at 18.2 — the `services`-oneof dispatch's `*ExtAuthz_GrpcService` switch-arm activates (replacing the 18.1 PARSE-REJECT) with `buildGRPCCheckFn`, per the ADR-0157 §Decision AMENDMENT. `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict; `initial_metadata` + `retry_policy` SILENT-IGNORED. The mode-agnostic `checkFn` closure signature stays `(ctx, *authRequest) → (checkDisposition, error)`; the `*authRequest` extension carries 10 new gRPC-mode fields (`remoteAddr` / `localAddr` / `tlsServerName` / `peerCertDER` / `listenerPrincipal` / `protocol` / `requestID` / `streamStartTime` / `perRouteContextExtensions` / `downstreamPrincipal`).

`buildAttributeContext` (in `attributes.go`) populates the gRPC `AttributeContext` per parent §5.P4 + §11.P4 in-session refinement: source/destination Peers; `request.http` map (pseudo-headers lowercased + included; HCM-injected `x-forwarded-proto` / `x-request-id` / `x-envoy-auth-partial-body` visible by `DecodeHeaders` time); `request.time` as `Timestamp`; `tls_session.sni` populated only when `include_tls_session: true` AND TLS connection; `source.certificate` populated only when `include_peer_certificate: true` AND client cert presented; `destination.principal` populated AUTOMATICALLY from the listener TLS cert (NOT gated; the §11.P4 in-session finding); `source.principal` via ADR-0144 `DownstreamPrincipal()` first-value; `context_extensions` merged listener+per-route. `mapGRPCResponse` (in `check.go`) handles the 4-arm `append_action` dispatch (per D5), the verbatim deny-header pass-through (UNLIKE HTTP-mode's matcher-filtered shape per parent §5.P11), and the envoy-go-strict treatment of `OkResponse+non-zero status` AND `DeniedResponse+zero status` as `dispError` per SPEC §6.7 commentary.

The differential fixture `0021-http-ext-authz-grpc` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 8 scenarios across a three-listener topology (l_test_a hosts allow/deny/body/per-route-disabled/per-route-check-settings/OkHttpResponse-mutation; l_test_b hosts error → `status_on_error`; l_test_c hosts `failure_mode_allow:true`). All 8 PASS at byte-exact body/status + cross-side counter-delta equivalence on the 5 reachable counters. The fixture is the FIRST envoy-go test to wire an in-process gRPC `Authorization/Check` server.

Six ADRs were touched (5 ADR numbers consumed, 2 BRAND-NEW): ADR-0157 §Decision AMENDMENT (Task 3); ADR-0158 full §Decision + §Consequences (Task 3); ADR-0160 gRPC-mode portion §Decision + §Consequences (Task 5); ADR-0161 gRPC-mode portion §Decision + §Consequences (Task 6); ADR-0165 callback-surface extension (Task 4 — NEW; D12 hypothesis HELD); ADR-0166 plaintext h2c upstream relaxation (Task 11.5 fixup — NEW; surfaced unanticipated at IMPL time). **The ADR-0044 escape-valve fired TWICE** in this phase — once at PLAN time (planner-decision D12 anticipated it; ADR-0165 landed at Task 4) + once at IMPL-fixup time (ADR-0166 landed at Task 11.5 fixup). Next-free ADR advances `ADR-0165 → ADR-0167`.

**Notable IMPL-time discoveries:**
- **Listener-principal sourcing required Outcome B plumbing** (Task 4 Step 0 pre-spike confirmed): the listener's `*stdtls.Config.Certificates[0]` is held only on the per-chain `chainInfo.tlsCfg` field at listener-build time and is NOT reachable from `connection.go:dispatchRequest`. A new `extractListenerPrincipal` helper in `listener/manager.go` lifts the listener-cert identity through `listenerCtx` → `hcm.ListenerCtx` → `*hcm.Filter.listenerPrincipal` (~+55 LoC across 5 files, within the PLAN budget).
- **Cluster-manager plaintext h2c upstream was rejected at master tip** (Task 11.5 fixup): the master-tip `Cluster.extractH2Mode` required TLS for any h2 cluster, but SPEC §7.2 + planner-decision D13 mandate plaintext h2c for fixture 0021. ADR-0166 lifted the gate (small-blast-radius; TLS+h2 path bit-identical; ADR-0044 escape-valve fired SECOND time).

---

## 2. Deliverables roster

**Production code:**
- `internal/grpcclient/` — NEW package (`doc.go` 86 + `grpcclient.go` 241 = 327 LoC production; `grpcclient_test.go` 820 LoC). Public surface: `Dialer` + `AuthClient` + `New(*cluster.Manager) *Dialer` + `(*Dialer).DialContext(ctx, clusterName) (*grpc.ClientConn, error)` + `NewAuthClient(d, clusterName, timeout) (*AuthClient, error)` + `(*AuthClient).Check(ctx, *CheckRequest) (*CheckResponse, error)` + `(*AuthClient).Close() error`. PARSE-REJECT for unknown cluster + `UseH2() == false`.
- `internal/filter/http/extauthz/extauthz.go` (+311) — `*authRequest` 10-field extension; `*Filter.streamStartTime` + `DecodeHeaders` entry capture; `perRouteContextExtensionsFor` helper + `dispatchOutboundCheck` seeding (10 fields total); `appendDispatch` enum extension + `headerKV.action` + `checkDisposition.upstreamDel`; `applyUpstreamMutations` extended for `upstreamDel` + the 4-arm `append_action` dispatch per D5.
- `internal/filter/http/extauthz/attributes.go` (+275) — `buildAttributeContext` + 5 helpers (`addressFromNetAddr`, `lowercaseHeaderMap`, `firstOrEmpty`, `bodyStringIfNotBytes`, `bodyBytesIfBytes`).
- `internal/filter/http/extauthz/check.go` (+383) — `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` + `buildGRPCCheckFn` real body; `*ExtAuthz_GrpcService` arm activation.
- `internal/filter/http/callbacks.go` (+74) — 6 new `DecoderFilterCallbacks` methods per ADR-0165 (`DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`).
- `internal/filter/http/chain.go` (+119) — 6 new chain primitives + 6 chain fields + 6 `*decoderCB` readers mirroring the existing `SetTLSPrincipals` / `tlsPrincipals` / `DownstreamPrincipal()` pattern.
- `internal/filter/hcm/connection.go` (+28) + `h2dispatch.go` (+73) + `config.go` (+29) + `filter.go` (+21) — H1 + H2 dispatch wire-in for the 6 new chain primitives.
- `internal/listener/manager.go` (+75) — `extractListenerPrincipal` helper + listener-principal plumbing through `listenerCtx`.
- `internal/cluster/manager.go` (~+30) + `dial_h2.go` (~+80) — relaxed `extractH2Mode` + dial path to permit plaintext h2c upstream (Task 11.5 fixup + ADR-0166).

**Test surface:**
- `test/helpers/extauthzgrpc/` — NEW in-process gRPC `Authorization/Check` test-helper (FIRST in-process gRPC server in envoy-go's test tree; ~210 LoC production + 290 LoC tests; scriptable `CheckResponse` values keyed by `:path`).
- `test/fixtures/0021-http-ext-authz-grpc/` — NEW differential fixture (8 scenarios; 3-listener topology; envoy.yaml 244 + envoy-go.yaml 218 + driver.go 1172 + expectations.yaml 314 + README.md 259).
- `internal/filter/http/extauthz/fuzz_test.go` (+412) — NEW `FuzzCheckResponseMapping` (the 23rd fuzzer) + `FuzzExtAuthzConfigParse` corpus extension with `grpc_service` variants.
- `internal/filter/http/extauthz/extauthz_test.go` (+2195) — Groups 10–14 NEW (parse-time, mapGRPCResponse, buildAttributeContext, context_extensions threading, callback-surface coverage).
- 10 sister-package `*_test.go` mock-conformance fixes (+~9 LoC each) to add zero-value stubs for the 6 new `DecoderFilterCallbacks` methods.

**Documentation:**
- `docs/envoy-go/DECISIONS.md` (+~294 LoC across 6 ADR edits — see §3 ADR roster).
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` (+~145 LoC) — the 8-edit bundle (§3 below).
- `docs/envoy-go/ROADMAP.md` (+4 LoC) — row 18.2 + row 18 both flipped `done`.
- `docs/envoy-go/STATE.md` (+12 LoC) — active-phase advanced to `(none — phase 18.2 + phase 18 both done; awaiting next §9 family-row brainstorm)`; next-free ADR `ADR-0167`.
- `docs/envoy-go/phases/18.2-ext-authz-grpc/PROGRESS.md` (+1387 LoC) — the 14-task ledger + cold-start preconditions + ADR roster + planner-decision register D1..D13.

---

## 3. ADR roster

Six ADRs touched (5 ADR numbers consumed; 2 brand-new). All landed in the IMPL session per ADR-0044 ADR-on-impl convention.

**ADR-0157 §Decision AMENDMENT** (Task 3 — `59ee8dd`). Replaces the 18.1 `*ExtAuthz_GrpcService` PARSE-REJECT stub with `buildGRPCCheckFn`; `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict (`"ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"`); `core.GrpcService.initial_metadata` + `retry_policy` SILENT-IGNORED; `compiledConfig` struct shape UNCHANGED (field-final at 18.1; gRPC-specific state captured in closure lexical scope per §6.5 step 5).

**ADR-0158** (Task 3 — `59ee8dd`; full §Decision + §Consequences body landed). `internal/grpcclient/` framework primitive — `Dialer` (cluster-name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())`) + `AuthClient` (typed `Authorization/Check` wrapper from go-control-plane v1.32.4 — no codegen); one `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair created at config-load time + shared across per-stream Check calls; leaks-on-exit MVP per planner-decision D2; per-Check `context.WithTimeout` per planner-decision D9; cross-phase-reusable for ext_proc + global_ratelimit per ADR-0158 §Consequences.

**ADR-0160 gRPC-mode portion** (Task 5 — `c651cf3`; in-place EXTENSION of the 18.1 HTTP-mode §Decision + §Consequences). `buildAttributeContext` in `attributes.go`; source/destination `Peer` per §11.P4 + ADR-0144 reuse; `request.http` per parent §5.P4 + §11.P4 in-session refinement (pseudo-headers lowercased + included; HCM-injected headers visible at DecodeHeaders); `request.time` as `Timestamp`; `tls_session.sni` gated by `include_tls_session` (per §11.P4 RATIFICATION); `source.certificate` gated by `include_peer_certificate`; `destination.principal` populated AUTOMATICALLY per §11.P4 (NOT gated); `context_extensions` merged listener+per-route; `encode_raw_headers` `header_map` arm DEFERRED per planner-decision D6; `metadata_context` + `route_metadata_context` populated as empty messages.

**ADR-0161 gRPC-mode portion** (Task 6 — `1a6e3c6`; in-place EXTENSION + Task 6 fixup `a5f2a89` restored an inadvertently-dropped ADR-0162 heading + `---` separator). `mapGRPCResponse` + `buildAllowDispositionGRPC` + `buildDenyDispositionGRPC` in `check.go`; `OkHttpResponse.headers` set/append per the 4-arm `append_action` dispatch (per D5); `OkHttpResponse.headers_to_remove` populated into new `upstreamDel []string` on `checkDisposition`; `OkHttpResponse.response_headers_to_add` SILENT-IGNORED per planner-decision D11 (decode-side-only shape); `DeniedHttpResponse.{status.code, body, headers}` extracted verbatim (NOT filtered through `allowed_client_headers` — UNLIKE HTTP-mode; per parent §5.P11); envoy-go-strict treatment of `OkResponse + non-zero status` AND `DeniedResponse + zero status` as `dispError` per SPEC §6.7 commentary; `validate_mutations` gating identical to HTTP-mode → `dispInvalid` + `invalid` counter.

**ADR-0165 (NEW)** (Task 4 — `d743514`; D12 hypothesis HELD; SPEC §13.5 + §6.5 step 5 + §6.6 AMENDED in-place). Cross-phase-reusable callback-surface extension to `DecoderFilterCallbacks` — adds 6 new accessor methods (`DownstreamRemoteAddr`, `DownstreamLocalAddr`, `DownstreamTLSServerName`, `DownstreamTLSPeerCertDER`, `DownstreamProtocol`, `ListenerPrincipal`); seeded at HCM-dispatch (H1 `connection.go` + H2 `h2dispatch.go`) via 6 new chain primitives mirroring the existing `SetTLSPrincipals` / `tlsPrincipals` / `DownstreamPrincipal()` pattern. ADR-0044 escape-valve fired FIRST time at PLAN time per planner-decision D3 + D12 — the SPEC §13.5 hard constraint "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" was in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION (populated `tls_session.sni`, `source.certificate`, socket addresses, `destination.principal`); the planner verified the alternative (UNPOPULATED fields) was a behaviorally-significant divergence vs reference Envoy. SPEC §13.5 + §6.5 + §6.6 were AMENDED in-place at the Task 4 commit with grep-archaeology preserving the original wording (line 2117 of BEHAVIOR_CONTRACT.md explicitly quotes the falsified original claim).

**ADR-0166 (NEW)** (Task 11.5 fixup — `55ca711`; surfaced unanticipated at IMPL time). Plaintext h2c upstream relaxation in `cluster.Manager.extractH2Mode` + `dial_h2.go` — the master-tip code unconditionally required TLS for any h2 cluster, but SPEC §7.2 + planner-decision D13 mandate plaintext h2c for fixture 0021. The fix is small-blast-radius: TLS+h2 path is bit-identical (the gate flipped from "require TLS for h2" to "allow plaintext h2c"); new tests for the plaintext h2c path; reference-Envoy parity. ADR-0044 escape-valve fired SECOND time. **Lesson:** the §11.P13 in-session SPEC scrape closure removed the most-likely ADR-0044 escape-valve surface (gRPC dial / TLS-to-auth-cluster plumbing) but could not anticipate the orthogonal cluster-manager-h2c-gate surface — the SPEC's pin-closure protects against ONE surface but cannot anticipate orthogonal ADR-0044 trigger surfaces.

**No ADR-0125 amendment.** ADR-0125's canonical-pattern roster stays at 8 entries after phase 18 (5th-canonical-REUSE recorded at 18.1 via ADR-0163; phase 18.2 adds no new canonical). `grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md` returns 0.

---

## 4. SPEC §15 acceptance checklist verification

Per SPEC §15. All 15 claims verified PASS with citations.

- [x] **Claim 1 — `internal/grpcclient/` package per ADR-0158.** **PASS.** `internal/grpcclient/{grpcclient.go, doc.go, grpcclient_test.go}` landed; `Dialer` + `AuthClient` types per §3.1 + §6 public surface; `grpc.WithContextDialer(cluster.Dial)` + `WithTransportCredentials(insecure.NewCredentials())` integration; PARSE-REJECT for unknown cluster + `UseH2() == false`; cross-phase-reuse forward-pointer in ADR-0158 §Consequences. Evidence: Tasks 2/3 PROGRESS entries; Gates A/B/D.

- [x] **Claim 2 — `grpc_service` arm activation per ADR-0157 §Decision AMENDMENT.** **PASS.** `*ExtAuthz_GrpcService` switch-arm in `buildCompiledConfig` calls `buildGRPCCheckFn` (NOT PARSE-REJECT); ADR-0157 §Decision amended in-place at Task 3; `core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict; `initial_metadata` + `retry_policy` SILENT-IGNORED. Evidence: Task 3 PROGRESS entry; Group 10 parse-time tests.

- [x] **Claim 3 — `buildGRPCCheckFn` per §6.5.** **PASS.** Cluster-manager lookup + `UseH2()` gate; `*grpcclient.AuthClient` constructed once at config-load time; closure captures `*AuthClient` + merged context_extensions + flags; per-Check timeout via `context.WithTimeout` per planner-decision D9. Evidence: Task 3 + Task 8 PROGRESS entries; `check.go` `buildGRPCCheckFn`.

- [x] **Claim 4 — `buildAttributeContext` per ADR-0160 gRPC-mode portion + §6.6.** **PASS.** Source/destination `Peer` per §11.P4; `request.http` populated set per parent §5.P4 + §11.P4 refinements (pseudo-headers lowercased + included; HCM-injected headers); `request.time` as `Timestamp`; `tls_session.sni` populated only when `include_tls_session: true` AND TLS connection; `source.certificate` populated only when `include_peer_certificate: true` AND client cert presented; `destination.principal` populated AUTOMATICALLY from listener TLS cert (NOT gated); `source.principal` via ADR-0144 `DownstreamPrincipal()` first-value; `context_extensions` merge listener+per-route. Evidence: Task 5 + Task 7 PROGRESS entries; Group 12 + Group 14 tests.

- [x] **Claim 5 — `mapGRPCResponse` per ADR-0161 gRPC-mode portion + §6.7.** **PASS.** OkResponse → allow with `OkHttpResponse.headers` set/append + `headers_to_remove`; DeniedResponse → deny with verbatim headers (NOT filtered through `allowed_client_headers`); empty CheckResponse → allow; non-zero status with no oneof → deny default 403; transport error → `dispError` + `failure_mode_allow` / `status_on_error` posture; `validate_mutations` gating identical to HTTP-mode → `dispInvalid` + `invalid` counter; envoy-go-strict treatment of `OkResponse+non-zero status` AND `DeniedResponse+zero status` as `dispError`. Evidence: Task 6 PROGRESS entry; Group 11 tests.

- [x] **Claim 6 — `compiledConfig` shape UNCHANGED.** **PASS.** No new field added (per §2.1 + ADR-0157 §Decision AMENDMENT); 18.2 swaps only the `checkFn` closure. Evidence: Task 3 PROGRESS entry; `extauthz.go` `compiledConfig` struct.

- [x] **Claim 7 — Per-route `context_extensions` consumption per §5.** **PASS.** The gRPC-mode-only field consumed proto-faithful; merged with listener-level (empty for MVP — `initial_metadata` deferred) at per-Check time; populates `AttributeContext.context_extensions`. Evidence: Task 7 PROGRESS entry; Group 14 tests; fixture scenario 7 received-CheckRequest assertion.

- [x] **Claim 8 — Empirical pins.** **PASS.** Parent §5 11 pins + §11.P4 + §11.P13 = all 13 pins CLOSED RATIFIED at the 18.2 SPEC commit per §11. 18.2 IMPL had zero RATIFIED-PENDING pins. Evidence: SPEC §11; parent SPEC §5; the end-to-end differential at Task 12 confirms the behaviors are preserved by the envoy-go implementation. See §5 below.

- [x] **Claim 9 — Differential fixture `0021-http-ext-authz-grpc` per §7.** **PASS.** 8 scenarios (7 mirroring 0020 + 1 gRPC-only `OkHttpResponse` mutation); three-listener topology (l_test_a/b/c); byte-exact body + status on allow + deny paths; cross-side counter-delta equivalence on 5 reachable counters (`ok` / `denied` / `error` / `failure_mode_allowed` / `invalid`); auth-server received-CheckRequest assertions including `context_extensions` content (scenario 7); 1 NEW test-helper `test/helpers/extauthzgrpc/`. Evidence: Tasks 9–12 PROGRESS entries; Gate E.

- [x] **Claim 10 — 23rd fuzzer per §7.3.** **PASS.** `FuzzCheckResponseMapping` at `internal/filter/http/extauthz/fuzz_test.go`; 30s ADR-0018 budget; existing 22 fuzzers re-run clean; `FuzzExtAuthzConfigParse` corpus extended with `grpc_service` variants. Evidence: Task 9 PROGRESS entry; Gate D.

- [x] **Claim 11 — BEHAVIOR_CONTRACT.md populated per Gate F.** **PASS.** 8-edit bundle landed: §13.1 ext_authz subsection's "gRPC mode — see phase 18.2" forward-pointers flipped to substantive gRPC content; §13.3 NEW row for 0021; §13.4 NEW `### Phase 18.2 forward-pointer notes`; NEW top-level `## gRPC client framework primitive (per phase 18.2 ADR-0158)` umbrella (the FOURTH top-level framework-primitive section); §13.2 stat-table UNCHANGED at 77; §13.5 `## HTTPFilterCallbacks` AMENDMENT (6 new accessors per ADR-0165 + grep-archaeology preservation of the falsified original claim); §13.4 cluster-manager amendment paragraph per ADR-0166; §11.P13 closure narrative. Evidence: Task 13 PROGRESS entry; Gate F greps green.

- [x] **Claim 12 — DECISIONS.md populated per ADR-on-impl convention.** **PASS.** ADR-0158 §Decision + §Consequences landed (Task 3); ADR-0157 §Decision AMENDMENT landed (Task 3); ADR-0160 + ADR-0161 gRPC-mode portion §Decision + §Consequences landed (Tasks 5/6); ADR-0165 NEW §Context + §Decision + §Consequences landed (Task 4); ADR-0166 NEW §Context + §Decision + §Consequences landed (Task 11.5 fixup). **The SPEC §10 anticipated "~0–1 impl-time-unanticipated ADRs"; 18.2 IMPL landed 2 (ADR-0165 + ADR-0166).** Evidence: DECISIONS.md `grep -cE '^## ADR-0165|^## ADR-0166'` = 2.

- [x] **Claim 13 — ROADMAP.md row 18.2 + parent row 18.** **PASS.** Row `18.2` flipped `in-progress → done` AT THE SAME COMMIT as parent row `18` flipped `in-progress → done` (Task 13 commit `620548ec`; date `2026-05-15`). The commit-message body explicitly names BOTH transitions for grep-verifiability. The phase-18 ADR-0045 split is now fully closed. Evidence: Task 13 PROGRESS entry; ROADMAP.md; commit `620548ec` message body.

- [x] **Claim 14 — All six phase-done gates green.** **PASS.** All 6 gates GREEN at `620548ec` — see §11 below. Evidence: Task 13 PROGRESS entry; Gate A–F outputs.

- [x] **Claim 15 — No master mutation outside the 18.2 squash-merge commit.** **PASS (pending merge).** All phase-18.2 IMPL work landed on the `phase-18.2-ext-authz-grpc-impl` worktree branch; master tip unchanged at `6226d11` until the `wt-merge` squash-commit + SHA-fill follow-up. Evidence: `git log --oneline master | head -1` = `6226d11`.

**Summary:** 15 claims PASS; 0 BLOCKED; 0 DONE_WITH_CONCERNS. The TWO impl-time-unanticipated ADR landings (ADR-0165 at PLAN time per D12; ADR-0166 at IMPL-fixup time) represent a deviation from the SPEC §10 "~0–1 impl-time-unanticipated ADRs" anticipation; both are recorded under §6 + §7 below.

---

## 5. Empirical-pin dispositions

All 13 parent SPEC §5 pins were RATIFIED at the 18.2 SPEC commit per SPEC §11 (in-session SPEC scrape on 2026-05-15 against reference Envoy v1.37.2 + go-control-plane v1.32.4). The 18.2 IMPL session had **zero RATIFIED-PENDING pins** — all closures happened at SPEC time.

- **§11.P4 (RATIFIED at SPEC time)** — `tls_session.sni` populated set + HCM-injected headers visible by DecodeHeaders time + `destination.principal` populated AUTOMATICALLY from listener TLS cert (NOT gated by `include_peer_certificate`). The IMPL's `buildAttributeContext` (Task 5) faithfully implements the populated set; the end-to-end differential at Task 12 (fixture 0021 scenarios 1/5/7) confirms the behaviors are preserved.
- **§11.P13 (RATIFIED at SPEC time)** — gRPC dial + TLS-to-auth-cluster plumbing via the cluster-manager. The §11.P13 closure REMOVED the most-likely ADR-0044 escape-valve surface (the cluster-manager coupling for `EnvoyGrpc` cluster-name resolution); ADR-0158 §Decision codifies the `grpc.WithContextDialer(cluster.Dial)` + `insecure.NewCredentials()` integration.

The §11 closures also surfaced the §11.P4 refinement that the planner did not anticipate at parent SPEC time: `destination.principal` populates AUTOMATICALLY from the listener TLS cert (NOT gated). This refinement is codified in ADR-0160 gRPC-mode portion §Decision.

The 11 parent §5 pins (§18.P1 / §18.P2 / §18.P3 / §18.P5 / §18.P6 / §18.P7 / §18.P8 / §18.P9 / §18.P10 / §18.P11 / §18.P12) all RATIFIED at the parent SPEC commit + closed at 18.1 IMPL (§18.P6 / §18.P7 / §18.P11) — see 18.1 REVIEW.md §3 for the closure narratives.

**Note on the §11.P13 closure vs ADR-0166.** The §11.P13 in-session scrape RATIFIED the gRPC dial / TLS-to-auth-cluster path against reference Envoy. The Task 11.5 fixup's plaintext h2c relaxation (ADR-0166) is an ORTHOGONAL surface — the cluster-manager's `extractH2Mode` gate was a pre-existing TLS-required-for-h2 invariant that fixture 0021 (plaintext h2c auth cluster per D13) required relaxing. The SPEC's pin-closure protects against ONE surface but cannot anticipate orthogonal ADR-0044 trigger surfaces — recorded as a lesson in §11 below.

---

## 6. Framework-delta impact + cross-phase reuse

Phase 18.2 introduces **ONE new framework primitive** + **ONE cross-phase-reusable callback-surface extension** + **ONE small-blast-radius cluster-manager relaxation** and **REUSES five** existing primitives per SPEC §3.

**ONE NEW primitive — `internal/grpcclient/` gRPC-client outbound framework primitive (ADR-0158).** envoy-go's FIRST gRPC infrastructure of any kind. Thin generic `Dialer` (cluster-name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` — TLS terminates at the cluster-manager layer) + ext_authz-typed `AuthClient` wrapper. Lives at `internal/grpcclient/` (mirroring `internal/matcher/` + `internal/jwks/`'s outside-`internal/filter/` placement) to anchor cross-phase reusability. Cross-phase-reusable for future ext_proc + global_ratelimit + any gRPC-family filter per ADR-0158 §Consequences.

**ONE cross-phase-reusable callback-surface extension (ADR-0165).** 6 new `DecoderFilterCallbacks` methods (`DownstreamRemoteAddr` / `DownstreamLocalAddr` / `DownstreamTLSServerName` / `DownstreamTLSPeerCertDER` / `DownstreamProtocol` / `ListenerPrincipal`) + 6 new chain primitives + 6 new chain fields + 6 new `*decoderCB` readers + HCM dispatch wire-in (H1 `connection.go` + H2 `h2dispatch.go`) + listener-principal plumbing (`extractListenerPrincipal` helper + `listenerCtx` field). REUSABLE by ext_proc + global_ratelimit + any future filter needing socket/TLS/listener-cert introspection at decode time. The seeding pattern mirrors the existing `SetTLSPrincipals` / `tlsPrincipals` / `DownstreamPrincipal()` pattern (ADR-0144), so the extension is a STRUCTURAL extension of an established pattern rather than a new architecture.

**ONE small-blast-radius cluster-manager relaxation (ADR-0166).** `Cluster.extractH2Mode` + `dial_h2.go` extended to permit plaintext h2c upstream clusters (the master-tip code unconditionally required TLS for any h2 cluster; SPEC §7.2 + planner-decision D13 mandated plaintext h2c for fixture 0021). Cross-phase-reusable by any future H2-upstream-only filter; the TLS+h2 path is bit-identical.

**FIVE REUSES per SPEC §3:**
- `internal/cluster.Manager` + `Cluster.Dial(ctx)` — REUSED via `grpc.WithContextDialer((*cluster.Cluster).Dial)` (the cluster-manager owns endpoint selection + TLS termination; gRPC layers framing on top).
- ADR-0144 `DownstreamPrincipal() []string` — REUSED for `AttributeContext.source.principal` (the first value per URI SAN → DNS SAN → CN priority); ext_authz gRPC-mode is the SECOND cross-phase consumer (after phase-16 rbac).
- Phase-09 async-resume primitive — REUSED via the `dispatchOutboundCheck` pattern (`StopIteration` + goroutine + `cb.ContinueDecoding()` on completion). Same primitive as 18.1's HTTP-mode `checkFn` invocation.
- Phase-13 ADR-0128 decode-side body-buffering — REUSED for `with_request_body` flow; body is attached to `AttributeContext.request.http.{body, raw_body}` per `pack_as_bytes`. ADR-0162's `pack_as_bytes` 18.2 honoring lands here.
- ADR-0085 `SendLocalReply` — REUSED for deny-path emission identical to HTTP-mode (other than the verbatim header pass-through per §4).

**No new ADR-0125 canonical.** ADR-0125's canonical-pattern roster stays at 8 entries.

---

## 7. Divergence-window roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 18.2 forward-pointer notes" + SPEC §8:

- **`OkHttpResponse.response_headers_to_add` SILENT-IGNORED per planner-decision D11.** The field PARSES; envoy-go does NOT inject these headers into the downstream RESPONSE on allow (the filter is decoder-only per ADR-0156; no encode-leg). Joint with the 18.1 `allowed_client_headers_on_success` deferral. Fuzz corpus + Group 11 unit test cover the silent-ignore path.
- **`encode_raw_headers` `header_map` arm DEFERRED per planner-decision D6.** The flag PARSES (no PARSE-REJECT); when set true, envoy-go does NOT populate `request.http.header_map` (the ordered alternative). Only the legacy `headers` map is populated. Byte-equivalent to reference Envoy at the AttributeContext level when `encode_raw_headers: false` (the default). Documented in BEHAVIOR_CONTRACT §13.4.
- **Envoy-go-strict treatment of `OkResponse + non-zero status` AND `DeniedResponse + zero status` as `dispError` per SPEC §6.7.** Reference Envoy is lenient with these structurally-inconsistent CheckResponses; envoy-go-strict classifies them as errors + applies `failure_mode_allow` / `status_on_error` posture. The fuzzer `FuzzCheckResponseMapping` exercises both classes.
- **`core.GrpcService.GoogleGrpc` arm PARSE-REJECTs envoy-go-strict.** Not a divergence-window per se — listed for surface-completeness. envoy-go uses `google.golang.org/grpc` directly; the `GoogleGrpc` arm (which configures a self-contained gRPC client with its own dial/TLS/retry semantics) is permanently out-of-scope. `core.GrpcService.EnvoyGrpc` is the supported arm.
- **`core.GrpcService.initial_metadata` + `retry_policy` SILENT-IGNORED** per SPEC §2.6 + §8 items 2+3. The fields PARSE; MVP does NOT thread initial_metadata into the per-Check context; gRPC client retry is a follow-up.
- **DeniedHttpResponse headers verbatim pass-through (UNLIKE HTTP-mode).** Per parent §5.P11 RATIFIED: gRPC mode passes auth-supplied deny-headers wholesale, including `content-type`, without filtering through `allowed_client_headers`. The fixture-0021 scenario 2 confirms byte-equivalence vs reference Envoy.
- **Go `net/http` User-Agent default-injection (router upstream-write boundary).** Not an ext_authz divergence; documented prophylactically in the fixture-0021 README as a known router-side quirk that may surface in non-ext_authz fixtures.

Fixture 0021 exercises all divergence-windows that have observable wire-shape; expectations.yaml carries explicit allow-list comments for each deferred item.

---

## 8. PLAN-time + IMPL-time deviations

**PLAN-time deviation: SPEC §13.5 AMENDMENT per planner-decisions D3 + D12.** The PLAN's planner-time settle of D3 + D12 hypothesized the callback-surface extension would be unavoidable: the SPEC §13.5 hard constraint "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" was in direct conflict with SPEC §15 item 4 + §11.P4 RATIFICATION (populated `tls_session.sni`, `source.certificate`, socket addresses, `destination.principal`); the planner verified the master-tip callback surface against the SPEC's required population set and confirmed the constraint was unsatisfiable. The hypothesis HELD at Task 4 — ADR-0165 fired. SPEC §13.5 + §6.5 step 5 + §6.6 were AMENDED in-place at the Task 4 commit (`d743514`) with grep-archaeology quoted-blocks preserving the original wording. BEHAVIOR_CONTRACT.md line 2117 carries the explicit FLIP narrative ("AMENDMENT to 18.1-anchored claim — §13.5 originally pinned 'NO new method on DecoderFilterCallbacks lands at 18.2'; that claim is FLIPPED at phase-18.2 PLAN time per planner-time decisions D3 + D12 + IMPL Task 4 landing.").

**IMPL-time deviation: ADR-0166 plaintext h2c upstream relaxation (Task 11.5 fixup).** The Task 11 implementer surfaced an unanticipated cross-task design concern at fixture-bootstrap time: envoy-go's cluster manager unconditionally required TLS for any H2 cluster (the master-tip `Cluster.extractH2Mode` invariant), but SPEC §7.2 + planner-decision D13 mandate plaintext h2c for fixture 0021. The ADR-0044 escape-valve fired a SECOND time at Task 11.5 fixup — ADR-0166 landed to relax `extractH2Mode` + `dial_h2.go`. The fix is small-blast-radius: TLS+h2 path is bit-identical (the gate flipped from "require TLS for h2" to "allow plaintext h2c"); new tests for the plaintext h2c path were added; reference-Envoy parity is preserved (reference Envoy accepts plaintext h2c upstream natively).

**Deviation counts vs SPEC §10 anticipation.** SPEC §10 anticipated "~0–1 impl-time-unanticipated ADRs"; 18.2 IMPL landed 2 (ADR-0165 + ADR-0166). The first (ADR-0165) was anticipated by planner-decision D12 as a strong hypothesis (so technically NOT impl-time-unanticipated — PLAN-time-anticipated); the second (ADR-0166) was genuinely impl-time-unanticipated. The §11.P13 in-session SPEC scrape closure was load-bearing — it removed the gRPC dial / TLS-to-auth-cluster escape-valve surface (the most-likely escape-valve at SPEC time) — but could not anticipate the orthogonal cluster-manager-h2c-gate surface that surfaced at fixture bootstrap.

---

## 9. Parent-rollup closure

**Phase 18 ADR-0045 split (18.1 + 18.2) is now fully closed.** Per parent SPEC §8 parent-rollup discipline:

- **Row 18.1** (`ext-authz-http`): `done` at `3cc8182` (2026-05-15).
- **Row 18.2** (`ext-authz-grpc`): `done` at `620548ec` (2026-05-15; Task 13).
- **Parent row 18** (`http-filter-ext-authz`): `done` at `620548ec` (2026-05-15; same commit as row 18.2 — per ADR-0106 + parent SPEC §8 parent-rollup discipline, the parent row closes AT THE SAME COMMIT as the final sub-row).

This is the phase-08-precedent rollup pattern (phase 08 split into 08.1+08.2+08.3 and closed all sub-rows + parent row at the same final-sub-phase commit per parent SPEC §8). The commit-message body for `620548ec` explicitly names BOTH transitions for grep-verifiability:

```
$ git log -1 --format=%B 620548ec | grep -E 'row 18'
phase 18.2 Task 13: ... ROADMAP row 18.2 in-progress→done + row 18 in-progress→done ...
```

The §9 HTTP-filters family now has 11 rows landed (phases 11–18 plus the 18.1/18.2 split counted as one family-row); 7 rows remain on the §9 roster (ext_proc / oauth2 / lua / wasm / adaptive concurrency / admission control / global rate limit) per ROADMAP.md.

---

## 10. Cross-phase reuse anticipation

The `internal/grpcclient/` primitive + the ADR-0165 callback-surface extension are STRATEGIC investments — both are intentionally minimal in surface area to support cross-phase reuse:

- **ext_proc (phase ~19+)** will REUSE `internal/grpcclient/Dialer` + the 6 new callback methods (DownstreamRemoteAddr / DownstreamLocalAddr / DownstreamTLSServerName / DownstreamTLSPeerCertDER / DownstreamProtocol / ListenerPrincipal). ext_proc composes its own typed wrapper (`*ProcessorClient` wrapping `envoy.service.ext_proc.v3.ExternalProcessor.Process` — bidi-stream, extending the Check unary pattern) on top of the shared `Dialer`. Per ADR-0158 §Consequences.
- **global_ratelimit (phase TBD)** will REUSE the same primitives. global_ratelimit composes `*RateLimitClient` wrapping `envoy.service.ratelimit.v3.RateLimitService.ShouldRateLimit` (unary, structurally identical to ext_authz's Check). Per ADR-0158 §Consequences.
- **Any H2-upstream-only filter** will REUSE the cluster-manager plaintext h2c relaxation per ADR-0166. The cluster-manager gate now treats h2c as a first-class transport rather than a TLS-only extension.

The `Dialer` surface is intentionally minimal — no future client coupling is anticipated to require `Dialer` API changes (per ADR-0158 §Consequences). The callback-surface extension is intentionally structurally-mirrored on the existing ADR-0144 `DownstreamPrincipal()` pattern — future filters can extend the chain primitives following the established seeding discipline.

---

## 11. Six-gate phase-done verification

Verbatim from PROGRESS.md Task 13 outputs (`620548ec`). All 6 gates GREEN.

**Gate A — build + vet + gofmt + lint clean:**
```
$ go build ./...        # exit 0
$ go vet ./...          # exit 0
$ gofmt -l .            # (empty — gofmt clean)
$ ~/go/bin/golangci-lint run   # exit 0
```
(Lint debt addressed during Task 13: 3 doc-comment instances of `marshalling`/`marshalled` flipped to American-spelling `marshaling`/`marshaled` to satisfy `misspell` linter; `attributes.go` 1 instance + `extauthz_test.go` 3 instances. Comment-only; no behavioral change.)

**Gate B — race tests across 88 packages, 0 FAIL:**
```
$ go test -race -count=1 ./...   # exit 0
ok  	github.com/esalaine/envoy-go/internal/grpcclient	1.083s
ok  	github.com/esalaine/envoy-go/test/differential	64.425s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzgrpc	1.063s
... (88 packages total — 40 with tests + 48 with [no test files])
```

**Gate C — h2spec 53/53 PASS at ADR-0051 pin:**
```
$ go test -v -count=1 -run '^TestH2Spec$' ./test/conformance/h2spec/
53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec (2.35s)
```
(Phase 18.2 introduces no H2 wire-shape changes; gate unchanged at the ADR-0051 pin. ext_authz uses gRPC over H2 to the upstream auth cluster, not to the downstream client.)

**Gate D — 23 fuzzers GREEN at 30s each:**
```
SUMMARY: PASS=23 FAIL=0
[PASS] internal/filter/http/extauthz :: FuzzCheckResponseMapping  (31.090s)  ← 23rd fuzzer, NEW at 18.2
[PASS] internal/filter/http/extauthz :: FuzzExtAuthzConfigParse   (31.074s)  ← corpus extended with grpc_service variants
... [21 pre-existing fuzzers all PASS]
```

**Gate E — 22 differential fixtures (0000–0021) PASS:**
```
$ go test -v -count=1 ./test/differential/
--- PASS: TestDifferential (59.91s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.60s)
    [21 pre-existing fixtures all PASS — see PROGRESS.md Task 13 output]
PASS
ok  	github.com/esalaine/envoy-go/test/differential	61.552s
```

**Gate F — BEHAVIOR_CONTRACT.md 8-edit bundle landed:**
```
$ grep -nE '^## gRPC client framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md
2191:## gRPC client framework primitive (per phase 18.2 ADR-0158)
$ grep -nE '^### Phase 18.2 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md
2448:### Phase 18.2 forward-pointer notes
$ grep -c '0021-http-ext-authz-grpc' docs/envoy-go/BEHAVIOR_CONTRACT.md   → 1
$ grep -c 'ADR-0165' docs/envoy-go/BEHAVIOR_CONTRACT.md                   → 8
$ grep -c 'ADR-0158' docs/envoy-go/BEHAVIOR_CONTRACT.md                   → 14
$ grep -c 'ADR-0166' docs/envoy-go/BEHAVIOR_CONTRACT.md                   → 10
$ grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md                  → 0   [NO §(xiv) — confirmed]
```

---

## 12. Lessons learned

**Concurrency lesson (Task 13 discovery): differential + fuzz must NOT run concurrently on a shared CPU pool.** The FIRST Gate-E run (interleaved with Gate D's 23-fuzzer workload) reported 2 spurious FAILs: `0017-http-bandwidth-limit` (wall-clock-sensitive — scenario1 reference run at 974ms, just under its 1.07s ceiling) and `0020-http-ext-authz-http` (auth-server backend timing-sensitive — scenario 7 with_request_body delay). Root cause: the fuzz workers (4–8 CPU threads per fuzzer, 23 fuzzers serialized but each saturating ~95% CPU during its 30s window) starved the Dockerized Envoy reference + the in-process echobackend / extauthzhttp goroutines. The Gate-B race-test run (which exercises the SAME differential suite under `-race`) ran clean WITHOUT concurrent fuzz workload. A subsequent Gate-E re-run after Gate D completed: 22/22 PASS in 61.552s, 0 FAIL. **Discipline lesson for future phase-done verification:** the gates must be serialized when running on a shared CPU pool. Recorded as a future-Phase note.

**§11.P13 pin-closure protects ONE surface but cannot anticipate orthogonal surfaces.** The §11.P13 in-session SPEC scrape RATIFIED the gRPC dial / TLS-to-auth-cluster plumbing path (the most-likely ADR-0044 escape-valve surface for 18.2 at SPEC time) — but could not anticipate the orthogonal cluster-manager-h2c-gate surface (ADR-0166) that surfaced at fixture bootstrap. **Lesson:** pin-closures protect against KNOWN escape-valve surfaces; the ADR-0044 escape-valve discipline must remain in reserve for ORTHOGONAL surfaces that surface at IMPL time. The phase-18.2 firing of ADR-0044 TWICE (once at PLAN time per D12 → ADR-0165; once at IMPL-fixup time → ADR-0166) is the first 18.x sub-phase to fire the escape-valve TWICE in one sub-phase — a useful precedent for future phases with complex framework-extension landings.

**The callback-surface extension was UNAVOIDABLE despite the SPEC's pin against it.** The SPEC §13.5 "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2" was a planner-time hope that was contradicted by SPEC §15 item 4 + §11.P4 RATIFICATION (populated set including socket addresses, TLS state, listener cert). The planner-time settle of D3 + D12 caught this BEFORE Task 4 implementation began, AMENDING SPEC §13.5 in-place at the Task 4 commit. **Lesson:** when SPEC pins are in tension with SPEC populated-set requirements, the PLAN-time settle must verify the master-tip surface against the populated-set requirement — and AMEND the SPEC pin if the populated-set is load-bearing. The grep-archaeology amendment discipline (preserve the falsified original wording verbatim + FLIP it in-place) is the canonical ADR-0044 amendment pattern.

**Sister-package test-mock extensions are required for compile-time interface conformance.** Extending the `DecoderFilterCallbacks` interface (the explicit Task 4 task) required every conforming type — including 10 test-mock types across `bandwidthlimit_test.go`, `buffer_test.go`, `compressor_test.go`, `csrf_test.go`, `extauthz_test.go` (`fakeExtAuthzDCB` + `asyncExtAuthzDCB`), `fault_test.go`, `header_mutation_test.go`, `jwtauthn_test.go`, `local_ratelimit_test.go`, `rbac_test.go` — to gain 6 new zero-value stub methods. Without this, the entire `./internal/filter/http/...` test surface failed to build. **Lesson:** PLAN file-lists list load-bearing PRODUCTION files; interface extensions implicitly extend ALL conforming types in the test surface. Future PLANs that extend a callback interface should explicitly enumerate the conforming-type test-mock extensions in the file-list.

**Listener-principal plumbing required Outcome B per Task 4 Step 0 pre-spike.** The listener's `*stdtls.Config.Certificates[0]` is held only on the per-chain `chainInfo.tlsCfg` at listener-build time + is NOT reachable from `connection.go:dispatchRequest`. The mitigation (an `extractListenerPrincipal` helper in `listener/manager.go` + a `listenerPrincipal` field plumbed through `listenerCtx` → `hcm.ListenerCtx` → `*hcm.Filter.listenerPrincipal`) was within the PLAN's +30–80 LoC budget for the listener-sourcing path. **Lesson:** PLAN-time LoC budgets that include sub-decision branches (Outcome A vs B) work well when the implementer verifies the outcome at Step 0 of the task — the +55 LoC actual was within budget because Step 0 confirmed Outcome B and the implementer didn't waste effort on Outcome A.

---

## 13. Sign-off

Phase 18.2 is **APPROVED and ready for master squash-merge via `wt-merge`** per the project memory `feedback_git_worktrees.md` + ADR-0003 worktree-isolation discipline. All 6 phase-done gates GREEN at HEAD `620548ec`; all 15 SPEC §15 acceptance claims verified PASS (0 BLOCKED, 0 DONE_WITH_CONCERNS); 5 ADR numbers consumed (ADR-0157 §Decision AMENDMENT + ADR-0158 full + ADR-0160 gRPC-mode portion + ADR-0161 gRPC-mode portion + ADR-0165 NEW + ADR-0166 NEW); NO ADR-0125 `**(xiv)**` amendment paragraph; ADR-0044 escape-valve fired TWICE (ADR-0165 at PLAN time per D12; ADR-0166 at IMPL-fixup time); 22 differential fixtures + 23 fuzzers GREEN; h2spec 53/53 at ADR-0051 pin; BEHAVIOR_CONTRACT 8-edit bundle landed; **ROADMAP row 18.2 + parent row 18 BOTH `done` at this commit** (2026-05-15) — the phase-18 ADR-0045 split is now fully closed; STATE.md at lifecycle-state-4 (`phase 18.2 done; phase 18 done; phase <next> BRAINSTORM pending`; next-skill: `superpowers:brainstorming`; next-free ADR: ADR-0167). The next session is a BRAINSTORM session for the next §9 HTTP-filters family-row from the remaining roster (ext_proc / oauth2 / lua / wasm / adaptive concurrency / admission control / global rate limit).

Phase-18.2 task chain summary: **14 tasks** at worktree HEAD (Tasks 1–13 + Task 11.5 IMPL-time fixup) including Task 4 SPEC §13.5 AMENDMENT (ADR-0165 callback-surface extension), Task 11.5 fixup (ADR-0166 plaintext h2c upstream relaxation), Task 6 fixup (ADR-0162 heading + separator restore), Task 10 fixup (extauthzgrpc helper extension), Task 13 6-gate verification + ROADMAP rollup. Phase-done six-gate verification at `620548ec`; phase-closed at this Task 14 commit. The last-commit SHA-fill is deferred to the post-`wt-merge` master-side follow-up per the phase-09..18.1 close pattern. The next session is a BRAINSTORM session for the next §9 HTTP-filters family-row.

**End of phase 18.2 review. Closes the parent phase-18 ADR-0045 split.**
