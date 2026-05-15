// Package extauthz implements envoy.filters.http.ext_authz — Envoy v1.37.2's
// canonical "external authorization" filter delegating the allow/deny decision
// to an external HTTP (or gRPC) service — under the 07.1 HTTP filter framework.
// Phase 18.1. The filter is the ELEVENTH §9 production HTTP filter and the FIFTH
// §9 row to ship pure decode-side (after csrf @ 12, buffer @ 13, rbac @ 16,
// jwt_authn @ 17).
//
// # Dual-mode service envelope
//
// The ExtAuthz `services` oneof resolves at config-load time into a `checkFn`
// closure on the `compiledConfig`. Two transport arms:
//
//   - `http_service`: consumed in 18.1. The filter POSTs the request to an
//     external HTTP authorization server and parses the HTTP response into a
//     `checkDisposition` ({allow, deny, error}).
//
//   - `grpc_service`: PARSE-REJECTS in 18.1 with `"ext_authz: grpc_service
//     mode not yet supported (lands in phase 18.2)"`. Phase 18.2 activates this
//     arm by amending ADR-0157's §Decision and wiring the `internal/grpcclient/`
//     primitive.
//
// An empty `services` oneof also PARSE-REJECTS (envoy-go-strict; the oneof
// carries no PGV `required` constraint, so the factory must reject it).
//
// # 18.1-consumed ExtAuthz fields (per parent SPEC §5.P1 + SPEC §1.1 amendments 1/2)
//
// Top-level (ExtAuthz proto, [#next-free-field: 30]):
//
//	Field 3  — http_service (*HttpService; consumed in 18.1)
//	Field 12 — transport_api_version (ApiVersion; V3-only PARSE-REJECT per ADR-0008)
//	Field 2  — failure_mode_allow (bool; default false)
//	Field 19 — failure_mode_allow_header_add (bool)
//	Field 5  — with_request_body (*BufferSettings; max_request_bytes > 0 required)
//	Field 6  — clear_route_cache (bool)
//	Field 7  — status_on_error (*type.v3.HttpStatus; default 403 when unset)
//	Field 24 — validate_mutations (bool)
//	Field 17 — allowed_headers (*matcher.ListStringMatcher; top-level request-side allow-list)
//	Field 25 — disallowed_headers (*matcher.ListStringMatcher; top-level request-side deny-list)
//	Field 13 — stat_prefix (string; consumed for SN2-reuse namespace)
//
// HttpService sub-proto (consumed):
//
//	server_uri, path_prefix, authorization_request, authorization_response
//
// Silent-ignored top-level fields (~13 per SPEC §2.2/§8):
// grpc_service, metadata_context_namespaces, typed_metadata_context_namespaces,
// route_metadata_context_namespaces, route_typed_metadata_context_namespaces,
// filter_enabled, filter_enabled_metadata, deny_at_disable,
// enable_dynamic_metadata_ingestion, filter_metadata,
// charge_cluster_response_stats, bootstrap_metadata_labels_key,
// emit_filter_state_stats, include_peer_certificate, include_tls_session,
// encode_raw_headers, decoder_header_mutation_rules.
//
// # Filter shape: DECODER-ONLY
//
// ext_authz is a pre-body request gate. The disposition is computed at
// DecodeHeaders time (with an async-resume leg for the outbound call + an
// optional ADR-0128 body-buffering wait when `with_request_body` is set).
// No encode-side surface is structurally needed; `Encoder: nil` in the
// HTTPFilter value. The compile-time assertion `var _ envoyhttp.StreamDecoderFilter`
// enforces this.
//
// Consequence: `allowed_client_headers_on_success` (which writes auth-service
// response headers to the downstream RESPONSE on the allow path) is DEFERRED
// per parent SPEC §5.P9 + §6 amendment 9 — a documented divergence-window.
//
// # Per-route: 5th-canonical REUSE (NO ADR-0125 §(xiv) amendment)
//
// `ExtAuthzPerRoute` carries a PGV-required `override` oneof with two arms:
//
//   - `disabled` (bool, PGV `const: true`): wholly deactivates the filter on
//     this route. envoy-go PARSE-REJECTs `disabled: false`. Per parent SPEC §6
//     amendment 7: `disabled: true` does NOT increment the `disabled` counter
//     (STRUCTURALLY UNREACHABLE under MVP — the disabled counter is the
//     runtime `filter_enabled` gate counter; see Stat surface below).
//
//   - `check_settings` (*CheckSettings): a NARROWER per-route override carrying
//     `context_extensions` (map[string]string; gRPC-mode-only — no HTTP-mode
//     effect in 18.1, documented no-op at SPEC §8 item 8), `disable_request_body_buffering`
//     (bool; overrides listener-level `with_request_body` to OFF), and
//     `with_request_body` (*BufferSettings; per-route body-buffering override;
//     mutually exclusive with `disable_request_body_buffering`).
//
// This maps onto ADR-0125's 5th canonical (disabled-bool arm + narrower
// override sub-message arm in a oneof). Phase 18 lands NO ADR-0125 amendment
// paragraph — the FIRST §9 row to REUSE an existing canonical. ADR-0163
// records the explicit no-amendment classification.
//
// Per-route stats are SHARED with listener-level (mirrors phase-12/13/14/17;
// DIVERGES from phase-11/15/16 INDEPENDENT-stats). The per-route override
// adjusts context_extensions / buffering but calls the same auth service;
// no new stateful policy-evaluation surface → no new stats.
//
// Pointer-identity `sync.Map` lazy-cache per ADR-0117 + ADR-0125 §(v).
//
// # 6-counter filterStats (per parent SPEC §6 amendment 8 + ADR-0156)
//
// All 6 counters registered unconditionally at New() time (predeclared empty
// counters for scrape stability per Prometheus best practice; mirrors
// phase-17 jwt_authn's unconditional-allocation discipline). Namespace shape
// per SN2-reuse: `http.<HCM_stat_prefix>.ext_authz.<counter>`.
//
//   - ok: request allowed by the auth service (HTTP 200).
//   - denied: request denied by the auth service.
//   - error: transport/timeout error or unrecognized auth status.
//   - disabled: runtime `filter_enabled` gate counter. STRUCTURALLY UNREACHABLE
//     under MVP (per parent SPEC §5.P12 / §6 amendment 7: `filter_enabled` and
//     `filter_enabled_metadata` are silent-ignored in 18.1; the counter
//     publishes 0 for the listener's lifetime). Registered for scrape-stability.
//   - failure_mode_allowed: error disposition + `failure_mode_allow:true`; both
//     `error` AND `failure_mode_allowed` increment.
//   - invalid: validate_mutations rejection → invalid counter; treated as the
//     error posture.
//
// # Async-resume outbound-call leg + per-request cancellable context
//
// The HTTP-mode auth-check is async per the phase-09 fault async-resume
// primitive: `DecodeHeaders` returns `StopIteration` synchronously; a goroutine
// performs the cancellable outbound POST; `cb.ContinueDecoding()` or a
// `SendLocalReply` fires on completion. `OnDestroy` cancels the in-flight
// call's `context.Context` (the FIRST §9 row with a per-request cancellable
// outbound call). The `filter` struct carries `mu sync.Mutex` + `done bool`
// + `callCtx context.Context` + `callCancel context.CancelFunc` for the
// race-guard (full wiring at Task 9; fields declared in skeleton at Task 2 per
// planner-time decision D4).
//
// # Deny / error wire shapes (per SPEC §4 + parent SPEC §5.P10/§5.P11)
//
//   - deny (auth service returned a recognized deny status 401/403):
//     `SendLocalReply(authStatus, verbatimBody, allowed_client_headers-filtered)`.
//     `content-length` synthesized per ADR-0085. `text/plain` content-type
//     fallback if auth service did not supply one.
//
//   - error + `failure_mode_allow:false` (default): `SendLocalReply(statusOnError, "", {})`.
//     Default `statusOnError` = 403.
//
//   - error + `failure_mode_allow:true`: allow through; if `failure_mode_allow_header_add`
//     also true, add `x-envoy-auth-failure-mode-allowed: true` upstream.
//     Both `error` AND `failure_mode_allowed` counters increment.
//
// # Public API
//
//   - TypeURL: canonical Envoy type-URL for the ext_authz HTTP filter config.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error):
//     the HTTPFilterFactory registered at boot per ADR-0072 (boot-registration
//     site in `cmd/envoy-go/main.go` alphabetical between `envoygotest` and
//     `fault` per ADR-0100 §2.2; lands at Task 10).
//
// # Divergence windows vs reference Envoy v1.37.2
//
//   - `allowed_client_headers_on_success`: DEFERRED (decode-side-only filter
//     shape has no encode leg; see SPEC §2.3 + §8 deferral 5).
//   - `response_code_details` on deny: NOT emitted (joint divergence-window
//     with phase-16 rbac + phase-17 jwt_authn per SPEC §2.8).
//   - Dynamic-metadata family (4 `*metadata_context_namespaces` fields +
//     `dynamic_metadata_from_headers` + `enable_dynamic_metadata_ingestion` +
//     `filter_metadata`): DEFERRED per SPEC §8 deferrals 2/3/7.
//   - Runtime family (`filter_enabled` / `filter_enabled_metadata` /
//     `deny_at_disable`): silent-ignored; `disabled` counter STRUCTURALLY
//     UNREACHABLE under MVP per SPEC §8 deferral 4.
//   - Cluster-scoped `cluster.<upstream>.ext_authz.*` stat triple: DEFERRED
//     per SPEC §8 deferral 6.
//   - `query_parameters_to_set` / `query_parameters_to_remove` (gRPC-mode
//     `OkHttpResponse` only): DEFERRED per SPEC §8 deferral 3.
//   - grpc_service mode: PARSE-REJECTS in 18.1; lands in 18.2.
//
// # ADR anchors (per ADR-0044 ADR-on-impl convention)
//
//   - ADR-0156: package shape + DECODER-only HTTPFilter + 6-base-counter
//     filterStats + unconditional allocation + deny-path SendLocalReply mechanism
//   - boot-registration ordering. Lands Task 2.
//   - ADR-0157: compiledConfig shape + services-oneof dual-mode dispatch
//     (grpc_service arm PARSE-REJECTS in 18.1; §Decision amended at 18.2) +
//     consumed-vs-deferred field discipline + error-posture fields +
//     transport_api_version V3-only PARSE-REJECT + empty-services factory
//     rejection + error-classification boundary. Lands Task 2.
//   - ADR-0159: HTTP-outbound auth-check framework primitive (thin ext_authz-
//     local client; disposition (b) per SPEC §3.1; no shared internal/httpclient/).
//     Lands Task 3.
//   - ADR-0160: AuthorizationRequest builder (HTTP-mode portion) — headers_to_add
//   - path_prefix prepend + allowed_headers/disallowed_headers filtering +
//     deprecated AuthorizationRequest.allowed_headers honored-if-present. Lands Task 4.
//   - ADR-0161: bidirectional header-mutation discipline (HTTP-mode portion) —
//     allowed_upstream_headers + validate_mutations + deny-path wire shape +
//     allowed_client_headers_on_success deferral. Lands Task 5.
//   - ADR-0162: request-body inclusion via phase-13 ADR-0128 reuse + over-limit
//     413 + connection:close edge case. Lands Task 6.
//   - ADR-0163: per-route 5th-canonical REUSE classification (NO ADR-0125 §(xiv)
//     amendment) + SHARED-stats + 6-counter stat surface. Lands Task 7.
//   - ADR-0125 §(v): 5th-canonical REUSE (the pattern phase-13 buffer introduced;
//     ext_authz is the FIRST §9 row to REUSE it).
package extauthz
