# envoy-go Behavior Contract

This file is the canonical reference for differential equivalence between envoy-go and the upstream Envoy image pinned in `ENVOY_TARGET.md` (see doctrine `D-3.3` and `D-3.7`). When a phase introduces new features, it extends the per-layer subsections below with concrete rules. When a phase's observed behavior diverges from this contract, either the contract is updated *via ADR* or the implementation is fixed — never both silently.

The contract is the contract. Do **not** consult Envoy C++ source to resolve ambiguity; settle ambiguities with an ADR in `DECISIONS.md` that extends the relevant subsection here.

---

## Equivalence Matrix (§7.2 of BOOTSTRAP_PROMPT.md)

| Dimension | Required equivalence |
|---|---|
| Response status | Exact |
| Response body | Byte-exact for deterministic handlers; semantically equal for filter-modified bodies |
| Response headers | Set-equal modulo documented allow-list (`server`, `date`, timing/identity headers explicitly listed) |
| Response trailers | Set-equal under the same allow-list discipline |
| HTTP/2 & HTTP/3 framing | Structurally equivalent (same frame types/order on equivalent events); not byte-equal |
| Access log records | Semantically equal after field-mapping |
| Stats output | Per-stat behavioral delta after defined load is equal between envoy-go and reference Envoy. Gauges are snapshot-equal after drain. Names + label keys + types byte-equal; HELP text ignored. Allow-list: 46 stats listed in § Stat-name mapping. All other Envoy stat names in /stats/prometheus output are ignored by the differential. |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |
| HTTP filter chain | Per-request equivalence on cors preflight + actual-request response shapes (status + header set + body) between envoy-go and reference Envoy. Filter iteration order, sendLocalReply encode-chain entry, and 413 overflow shape are verbatim-pinned at the ENVOY_TARGET SHA. Differential covers cors only; `envoy.filters.http.envoy_go_test` excluded (test-only); other filters in the §9 family are future-phase scope. |
| Listener filters | Per-connection chain-selection equivalence: which `filter_chain` is dispatched is byte-equal across envoy-go and reference Envoy. Verified via per-connection backend-port routing in fixture 0008. Chain-match precedence ordering, `default_filter_chain` fallback semantics, and empty-match-vs-default resolution are verbatim-pinned at the ENVOY_TARGET SHA. Differential covers chain-selection only (which backend each connection is routed to); listener-filter internal byte-level behavior (e.g., tls_inspector parser output) is unit-tested only. |
| Admin /config_dump | Body byte-equal modulo build/timestamp/uptime allow-list. Three-envelope ordering: Bootstrap, Listeners, Clusters. Allow-list: `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated` per-field allow-listed. dynamic_* arrays absent in both. (Per phase 08.1 SPEC §13.2.) |
| Admin /clusters | Tuple-set equality on `(cluster, key, value)` triples. envoy-go emits Envoy's full unconditional 28-line-per-cluster + 18-line-per-endpoint set with default constants for non-modeled fields. Allow-list: hot-path counters `cx_total`, `cx_connect_fail`, `rq_total`, `rq_active`, `rq_error` allow ±1 tolerance. (Per phase 08.1 SPEC §13.2.) |
| Admin /listeners | Body byte-equal (after framing dechunk). Single line per listener. No allow-list. (Per phase 08.1 SPEC §13.2.) |
| Admin /server_info | Body byte-equal modulo build/uptime/CLI-flags/node allow-list. The `state` field IS asserted byte-equal. Allow-list: `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset), `hot_restart_version`, `node.user_agent_*`, `node.extensions[]` per-field allow-listed. (Per phase 08.1 SPEC §13.2.) |
| Admin /drain_listeners | Body byte-equal `OK\n` (POST); 405 + body `Method <X> not allowed, POST required.\n` (non-POST). Idempotent semantics; query-param ?graceful=true silent-ignored. Header set inherits umbrella rules; framing per phase-01 dechunk-discipline. Method-discrimination is the FIRST envoy-go endpoint with 405 enforcement (per ADR-0093 partially amending ADR-0090). |
| Admin /ready (DRAINING) | Body byte-equal `DRAINING\n` to reference Envoy v1.37.2 in DRAINING state. Status 503. DRAINING precedence over LIVE / PRE_INITIALIZING. Status 503 (matches PRE_INITIALIZING). Header set inherits umbrella rules. Per-proxy trigger script normalization per 08.2 SPEC §7.2 (envoy-go: /drain_listeners; ref Envoy: /drain_listeners + /healthcheck/fail). |
| Admin /server_info (DRAINING) | The `state` field IS asserted byte-equal (`"DRAINING"`) when both proxies are in DRAINING. Other fields per ADR-0088 allow-list. Inherits ADR-0088 allow-list for non-state fields (version, uptime_*, command_line_options, hot_restart_version, node). |
| HTTP filter `envoy.filters.http.fault` | Per-request equivalence on abort response shape (status + 4-header set + body byte-exact `fault filter abort`), delay timing (±10ms tolerance), per-route wholesale-override resolution, headers-field exact-match gate, and stat counter increments under the per-scenario differential gate (fixture 0011-http-fault). NOT asserted: header-driven fault path (deferred — ADR-0104), response_rate_limit (deferred), abort.grpc_status (deferred), HeaderMatcher non-exact variants. |
| HTTP filter `envoy.filters.http.header_mutation` | Per-request equivalence on post-mutation request headers (visible at upstream backend) and post-mutation response headers (visible at downstream client) under listener-level + per-route 3-tier configurations, including AppendAction × 4 + Remove + `keep_empty_value` boundary + multi-valued header collapse / preserve semantics + `most_specific_header_mutations_wins` cross-tier ordering (both flag values). Boot-time enforcement of the 6-name protected-header set per ADR-0111 + phase 10 §11.1. Differential gate fixture 0012-http-header-mutation. NOT asserted: header-value formatter substitution (deferred — ADR-0113), `query_parameter_mutations` (deferred — ADR-0112), H2 differential coverage. |
| HTTP filter `envoy.filters.http.local_ratelimit` | 0013-http-local-ratelimit: scenario1: 5 reqs / cap=10 / fill=10 / interval=1s — 5×200; scenario2: 5 reqs / cap=2 — 2×200 + 3×429 (§11.3 wire shape); scenario3: 3 reqs / cap=1 / fill=1 / interval=200ms (refill ±10ms per §11.7); scenario4: 3+3 reqs interleaved /strict + /loose — wholesale-override (§11.6). Per-scenario tolerance per §13.3 timing-tolerances; lowercase wire-form 4-header set on 429; counter deltas across 4 stat names with `envoy_local_http_ratelimit_prefix` Prometheus label. NOT asserted: descriptor-action path (deferred — ADR-0120 family), runtime shadow-mode (deferred), X-RateLimit headers (deferred), H2 differential coverage. |
| HTTP filter `envoy.filters.http.csrf` | 0014-http-csrf: scenario1: same-origin POST → 200; scenario2: cross-origin POST → 403 (§11.10 wire shape: content-length=14, body=`Invalid origin`, 4-header lowercase set); scenario3: `additional_origins` host:port match → 200; scenario4: no source-origin → 403 + `missing_source_origin +1`; scenario5: Referer fallback → 200; scenario7: per-route TPFC wholesale-override (§11.9 — per-route data REPLACES listener data; counters AGGREGATE since stats are SHARED). All 6 scenarios HTTP/1.1 plaintext; no timing tolerances (csrf is purely synchronous). NOT asserted: StringMatcher non-exact variants (deferred — drop at PARSE per ADR-0101 §3), `filter_enabled` percentage values other than 100% (deferred — Runtime + hot restart family), `shadow_enabled` semantics (deferred), H2 differential coverage. |
| HTTP filter `envoy.filters.http.buffer` | 0015-http-buffer: scenario1: body-fits-cap (1 KiB POST → 200); scenario2: streaming-overflow CL-known (2 MiB POST → 413; §11.7+§11.8 wire shape: content-length=17, body=`Payload Too Large`, 4-header lowercase set + Connection: close); scenario3: chunked-overflow against per-route tighter cap (200 KiB chunked → 413, NO 100-Continue with chunked); scenario4: per-route disabled bypass (2 MiB POST → 200 — cap wholly inactive on disabled route per §11.4); scenario5: per-route tighter override fires (200 KiB → 413 against 128 KiB override); scenario6: chunked-passthrough Content-Length injection (10 KiB chunked → 200, backend asserts inbound `Content-Length: 10240` per §11.8-CL `maybeAddContentLength` mirror). All 6 requests HTTP/1.1 plaintext; no timing tolerances (buffer is purely synchronous). Counter delta on envoy-go side: `downstream_rq_total +6`, `downstream_rq_2xx +3`, `downstream_rq_4xx +3`. Envoy-only `downstream_rq_too_large` (+3) and `downstream_rq_completed` (+6) filtered out via the existing twin-series-discipline allow-list. NOT asserted: `max_request_bytes > 1 MiB` operational behavior (deferred — envoy-go-only parse-time rejection per ADR-0126); `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` (silent-ignored per ADR-0076); H2 differential coverage. |
| HTTP filter `envoy.filters.http.compressor` | 0016-http-compressor (gzip-only response-side): byte-exact status; decompressed-byte-exact body on compressed scenarios per ADR-0133 (gzip compressed bytes are STRUCTURALLY non-byte-exact between Go `compress/gzip` and Envoy libz — multi-encoding gzip-format spec admits both); allow-list `content-length` value + `transfer-encoding` presence on compressed scenarios (envoy-go fixed-CL identity vs Envoy chunked per ADR-0131); per-counter delta byte-equivalent on 12 active counters (10 cross-side + 1 dynamic per-side `response_total_uncompressed_bytes` + 1 boundary-only `response_total_compressed_bytes` per ADR-0133 §Decision (iii)); 4 per-side empirical-divergence counters (`header_compressor_used`, `header_not_valid`, `response_not_compressed`, `request_not_compressed`) pinned per-side at Task 14 empirical evidence — both sides locked, regressions on either surface immediately. 6 scenarios per phase 14 SPEC §7.1 (compress-text-default, skip-content-type, skip-min-content-length, skip-on-etag, per-route-disabled, per-route-rmAE-override). HCM `directResponseAction.response_headers_to_add` plumbed via ADR-0134 with explicit `OVERWRITE_IF_EXISTS_OR_ADD` AppendAction parse-gate. NOT asserted: brotli + zstd codec extensions (deferred — extends ADR-0130); `request_direction_config` activation + the future `envoy.filters.http.decompressor` filter (deferred); chunked-encoded response wire shape on the encode side (deferred — couples to future encode-side streaming framework phase per ADR-0131); Gzip codec sub-knobs `memory_level` / `window_bits` / `chunk_size` (Go gzip does not expose libz equivalents); `choose_first` first-acceptable selection mode (deferred); `runtime_enabled` + `enabled` runtime gates (Runtime + hot restart family); deprecated top-level mirrors; per-route `overrides.compressor_library` library swap; H2 differential coverage. |
| HTTP filter `envoy.filters.http.bandwidth_limit` | 0017-http-bandwidth-limit (BOTH-direction Path B-async with kbps-per-tick throttle math; KiB/s units): byte-exact status; byte-exact body (bandwidth_limit does not transform bytes); **±70ms per-side wall-clock tolerance per scenario** per ADR-0137 wire-shape-divergence-window — phase 15 Task 14 empirically refuted the SPEC §11.P9(c) cross-side "total-throttle-time converges within ±70ms" claim for bodies within initial-burst capacity (Envoy's initial-burst-discount + per-request bump-on-active-side-regardless-of-body diverges from envoy-go's deterministic ceil-formula); each side asserted independently within ±70ms of its predicted target. Per-counter delta byte-equivalent on 6 active counters (`*_incoming_total_size`, `*_allowed_total_size` × {request, response} via cross-side delta-equal; `*_enabled` + `*_enforced` × {request, response} via `counterModePerSideExact` per the same initial-burst-discount divergence-window). 6 gauges per stat_prefix (`*_pending`, `*_incoming_size`, `*_allowed_size` × {request, response}) NOT asserted (transient/noisy mid-stream observations). 2 unconditional Envoy histograms (`request_transfer_duration`, `response_transfer_duration`) allow-listed via twin-series-filter divergence-window per §13.4 + `### Twin-series filter discipline` phase-15 extension. INDEPENDENT per-route stats per ADR-0139 (per-route override allocates own `*compiledConfig` + own `*filterStats` keyed by per-route `stat_prefix`). 6 scenarios per phase 15 SPEC §7.1 (response-only throttle, request-only throttle, REQUEST_AND_RESPONSE symmetric, tiny-body within-burst, per-route DISABLED-via-`enable_mode`, per-route override with own stat_prefix). NOT asserted: intra-throttle-window chunk-arrival timing (envoy-go Path B-async silent-then-blast vs Envoy Path A rate-paced chunks at `fill_interval` cadence — deliberate divergence per ADR-0137; couples to future encode-side streaming framework phase); the 4 trailer-mode trailers emitted when `enable_response_trailers: true` (deferred — couples to future trailer-emission framework phase); the 2 unconditional histograms above (twin-series-filter allow-listed); `runtime_enabled` runtime-gate paths (Runtime + hot restart family); H2 differential coverage; `BandwidthLimitPerRoute` wrapper proto (does not exist in Envoy v1.37.2 — per-route uses bare `BandwidthLimit` via TPFC per §11.P1 + ADR-0125 §(xi) NEW 6th canonical). |
| 0018-http-rbac | envoy.filters.http.rbac (decode-side dual-engine policy gate; rules-engine + matcher-engine + shadow + per-policy stats) | byte-exact status; byte-exact body on allow (passthrough) AND deny (19-byte "RBAC: access denied"); per-counter delta byte-equivalent on 4 base counters per active namespace (allowed/denied/shadow_allowed/shadow_denied); INDEPENDENT per-route stats per ADR-0145 (scenario 8); mTLS scenario 6 exercises ADR-0144 TLS-principal accessor; 7th canonical per-route absent-implies-disabled per ADR-0125 §(xii) (scenario 7) |
| 0019-http-jwt-authn | envoy.filters.http.jwt_authn (decode-side pre-body JWT bearer-token validation gate; RS+ES algorithm family; RemoteJwks + LocalJwks; Full-6 JwtRequirement; 8th-canonical per-route string-reference-delegation) | byte-exact body on allow paths (echo passthrough) AND deny paths (canonical jwt_verify_lib strings — `Jwt is missing` 14B / `Jwt is expired` 14B / `Jwt verification fails` 22B); status byte-exact (401 default, 403 for JwtAudienceNotAllowed per §11.P1); WWW-Authenticate header byte-exact including conditional `, error="invalid_token"` append per §11.P2 (driver pins `Host: jwt-authn.fixture.test` so the full-URL realm is per-side byte-equivalent); cross-side counter-delta equivalence on `denied` + `cors_preflight_bypassed` + `jwks_fetch_success` + `jwks_fetch_failed`; `allowed` asserted per-side (ref 5 / subj 3 — reference Envoy increments `allowed` on the CORS-bypass + per-route-disabled passthrough paths; envoy-go MVP per SPEC §3 + §1.1 amendment 5 does not — documented divergence-window); 2 `jwt_cache_*` counters STRUCTURALLY UNREACHABLE under MVP, not asserted; `response_code_details` NOT asserted (envoy-go MVP defers per §1.1 amendment 11); SHARED per-route stats per ADR-0154; 8th canonical per-route string-reference-delegation per ADR-0125 §(xiii) (scenarios 7 + 8) |
| 0020-http-ext-authz-http | envoy.filters.http.ext_authz (decode-side external-authorization gate; HTTP service mode; 5th-canonical per-route REUSE + SHARED-stats; ADR-0156/0157/0159/0160/0161/0162/0163) | byte-exact status; byte-exact body on allow paths (echo passthrough) AND deny paths (verbatim auth-service body — scenario 2 `"access denied"` 13B); `response_code_details` NOT asserted (envoy-go MVP defers per SPEC §2.8 + §8 item 10); cross-side counter-delta equivalence on 5 actively-emitted counters (`ok`, `denied`, `error`, `failure_mode_allowed`, `invalid`); `disabled` counter STRUCTURALLY UNREACHABLE, not asserted; SHARED per-route stats per ADR-0163 (5th-canonical-REUSE; no per-route `*filterStats`); per-route `disabled:true` arm (scenario 6) + `check_settings` arm (scenario 7) exercised; deny-path header-set ordering (decision headers first, framework housekeeping after) confirmed RATIFIED per §18.P11 at Task 13 |
| 0023-http-ext-proc-body | envoy.filters.http.ext_proc (BOTH-decode-and-encode external-processor filter; **body-stage activation — BUFFERED mode**; gRPC bidi-stream; 5th-canonical per-route REUSE + SHARED-stats; 9-counter MVP roster UNCHANGED at 19.2 + the 8 reference-only counters DEFERRED to 19.3+; ADR-0175 NEW encode-side `BufferEncodedBody` primitive + ADR-0168 §Decision AMENDMENT body-mode PARSE-REJECT lift + ADR-0171 §Decision AMENDMENT 4-stage state machine + per-message timer behavioral + ADR-0172 §Decision AMENDMENT `body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse`) | byte-exact status + body on allow + deny paths per the 7-sub-scenario matrix (a/b/c/d/e/f_a/f_b per fixture 0023 README); body-stage `body_mutation{body}` + `body_mutation{clear_body}` apply on envoy-go (race-tested + unit-tested at phase-19.2 IMPL Tasks 6+8); reference Envoy v1.37.2 body-stage `body_mutation` returns 500 — scenarios (b)+(d) re-scoped to OBSERVABILITY-only at the differential gate per fixture 0023 README **Empirical-pin AMENDMENT (I)** (root-cause analysis + closure deferred to a future phase); body-stage `ImmediateResponse` scenario (c) re-routed from response_body → request_body per fixture 0023 README **Empirical-pin AMENDMENT (II)** (envoy-go HCM rejects encode-side `SendLocalReply` after encode chain has started; the well-supported decode-side path delivers the substantive 403 contract); `CONTINUE_AND_REPLACE` at response_headers with body-mode BUFFERED scenario (e) combined header+body replacement; per-route body-mode override scenario (f) covers `f_a` (per-route override active) + `f_b` (per-route fallback to listener default); cross-side counter-PRESENCE-check equivalence on the 9 MVP counters; SHARED per-route stats per ADR-0173 (no per-route stat split at body-mode activation); processor-server received-`ProcessingRequest` body-stage envelope assertions confirming D5 attribute-roster HOLDS (body-stage CEL roster MIRRORS header-stage roster + `request.size`/`response.size` body-stage-natural additions per fixture 0023 driver `findBodyEnvelope`); three-listener topology REUSED from 0022; `test/helpers/extprocgrpc/` test-helper REUSED UNMODIFIED per SPEC §7.1; 6 scenarios per SPEC §7.2 (7 sub-scenarios counting f-a + f-b); 25th fuzzer `FuzzProcessingResponseMapping` exercising `*ProcessingResponse` dispatch surface (body-stage `body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse` arms) at the 30s ADR-0018 budget |
| 0022-http-ext-proc-grpc | envoy.filters.http.ext_proc (BOTH-decode-and-encode external-processor filter; **headers-stages-only mode** — body-stage activation lands at phase 19.2; bidi-stream gRPC `Process` RPC primary mode + JSON-transcoded HTTP POST secondary mode; 5th-canonical per-route REUSE + SHARED-stats; 9-counter MVP roster + 8 reference-only counters DEFERRED to 19.2; ADR-0167/0168/0169/0170/0171/0172/0173/0174) | byte-exact status; byte-exact body on allow paths (echo passthrough with applied CommonResponse.header_mutation) AND deny paths (ImmediateResponse body VERBATIM); cross-side counter-PRESENCE-check equivalence on the 9 actively-emitted MVP counters (per ADR-0173 §Consequences relaxation — per-scenario delta divergences DEFERRED to 19.2 under the §19.P4 RATIFIED-WITH-AMENDMENT authorization); SHARED per-route stats per ADR-0173 (5th-canonical-REUSE; mode-agnostic — per-route `grpc_service` overrides allocate own *ProcessorClient but do NOT spawn per-route stat split); processor-server received-`ProcessingRequest` assertions (the SECOND §9 row to assert outbound structured-proto content after phase-18.2 ext_authz-gRPC): request_headers stage carries `request_attributes` envelope populated per SPEC §6.6 (CEL attribute names mapping to envoy-go callback accessors via attributes.go); response_headers stage carries `response_attributes` envelope; HCM-injected headers (`x-envoy-internal`, `x-forwarded-proto`, `x-request-id`) visible in the per-stage header map. 8 scenarios (gRPC allow + header set, gRPC immediate_response at request_headers, gRPC response_headers mutation, gRPC mode_override mid-stream, gRPC failure_mode_allow, HTTP allow headers-only, per-route disabled, per-route processing_mode override); three-listener topology (l_test_a allow/deny/headers/per-route, l_test_b error→failure_mode_allow, l_test_c HTTP-mode); 1 NEW test-helper `test/helpers/extprocgrpc/` (FIRST in-tree bidi-stream gRPC `ExternalProcessor.Process` server; plaintext h2c on ephemeral port; scriptable per-`:path` ProcessingResponse sequence per stage); 24th fuzzer `FuzzExtProcConfigParse` (24 seeds covering both modes + all 18 PARSE-REJECT branches per SPEC §7.3 corpus) |
| 0021-http-ext-authz-grpc | envoy.filters.http.ext_authz (decode-side external-authorization gate; **gRPC service mode**; ADR-0157 §Decision AMENDMENT + ADR-0158/0160-gRPC/0161-gRPC/0165/0166 — 5 new ADRs land at 18.2; the §11.P13 in-session SPEC RATIFICATION removed the most-likely TLS-layer escape-valve and the §11.P4 RATIFICATION pinned the populated `AttributeContext` set including auto-populated `destination.principal`) | byte-exact status; byte-exact body on allow paths (echo passthrough) AND deny paths (`DeniedHttpResponse.body` VERBATIM); deny-path response headers VERBATIM (UNLIKE HTTP-mode's `allowed_client_headers`-filtered set — the central wire-shape contrast between the two modes); cross-side counter-delta equivalence on 5 actively-emitted counters (`ok`, `denied`, `error`, `failure_mode_allowed`, `invalid`); `disabled` counter STRUCTURALLY UNREACHABLE, not asserted; SHARED per-route stats per ADR-0163 (5th-canonical-REUSE; mode-agnostic — the `context_extensions` consumption in gRPC mode does NOT spawn per-route stat split); auth-server received-`AttributeContext` assertions (the FIRST §9 row to assert the outbound auth request's structured-proto content): pseudo-headers lowercased + present, HCM-injected headers (`x-envoy-auth-partial-body` / `x-forwarded-proto` / `x-request-id`) visible, `request.time` populated, `source/destination.address.socket_address` populated, `source.principal` from ADR-0144 `DownstreamPrincipal()`, `destination.principal` auto-populated from listener TLS cert (per §11.P4), `tls_session.sni` gated by `include_tls_session`, `source.certificate` gated by `include_peer_certificate`, `context_extensions` merge (listener+per-route) on scenario 7; 8 scenarios (7 mirroring 0020 + 1 gRPC-only `OkHttpResponse` upstream mutation per scenario 8); three-listener topology (l_test_a allow/deny/body/per-route, l_test_b error→`status_on_error`, l_test_c `failure_mode_allow`); 1 NEW test-helper `test/helpers/extauthzgrpc/` (FIRST in-process gRPC server in envoy-go's test tree; plaintext h2c on ephemeral port; scriptable per-`:path` CheckResponse); 23rd fuzzer `FuzzCheckResponseMapping` |

"Semantically equal" is defined per dimension in the subsections below. Where a dimension has no subsection yet, the matrix row is its complete definition and phases may only tighten (not relax) it.

---

## Header allow-list

The allow-list enumerates response headers whose values are permitted to differ between envoy-go and upstream Envoy without constituting a differential failure. Each entry names the header, the permitted divergence (presence-only / format-only / value-range), the phase that introduced the entry, and the ADR that justifies it.

| Header | Scope | Permitted divergence | Introduced by | Justifying ADR |
|---|---|---|---|---|
| `date` | Admin `/ready` response | Value is RFC 7231 IMF-fixdate, non-deterministic per request. Presence required on both upstream and subject responses; value NOT byte-compared. | Phase 01 | ADR-0015 |
| `Server` | HCM-locally-generated responses | Presence-only — Envoy: `envoy`; envoy-go: `envoy` (matches per ADR-0014 reaffirmation). Set-equality only. | Phase 04 | ADR-0044 |
| `Content-Length` | HTTP/1.1 responses | HTTP/1.1 framing-divergence-permitted (Envoy may choose Content-Length while envoy-go chooses Transfer-Encoding: chunked or vice versa; `http.ReadResponse` decodes both transparently). | Phase 04 | ADR-0044 |
| `Transfer-Encoding` | HTTP/1.1 responses | HTTP/1.1 framing-divergence-permitted (mirror of Content-Length). | Phase 04 | ADR-0044 |
| `x-envoy-*` | Routed-to-upstream HTTP/1.1 responses | Every header with this prefix; presence-not-required on subject (envoy-go does not inject these in phase 04; Envoy does). | Phase 04 | ADR-0044 |
| `x-forwarded-*` | Routed-to-upstream HTTP/1.1 responses | Every header with this prefix; presence-not-required on subject. | Phase 04 | ADR-0044 |
| `x-request-id` | Routed-to-upstream HTTP/1.1 responses | Presence-not-required on subject. | Phase 04 | ADR-0044 |
| `:status` | HCM-locally-generated H2 responses | Required + value-asserted | Phase 05.1 | ADR-0052 |
| `:method` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:path` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:scheme` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `:authority` | Routed-to-upstream H2 requests | Required + value-asserted (active per ADR-0057; phase 05.2) | Phase 05.1 | ADR-0052 |
| `x-ratelimit-limit` | HCM-locally-generated + routed-to-upstream responses (ratelimit filter encode side) | **Set-equal byte-exact** — NOT an allow-list "permitted divergence" entry but a set-equal-byte-exact entry per scenario (g) `x_ratelimit_headers` cross-side dispatch (parent SPEC §4.7 + AMEND-8; DRAFT_VERSION_03 byte format `<MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` per `ratelimit_headers.cc:13-65`). Emitted on ALL dispositions (OK + OVER_LIMIT + fail-open error) when `enable_x_ratelimit_headers == DRAFT_VERSION_03`. The fail-closed reply path does NOT emit (nullptr-mutate per AMEND-8). | Phase 24.2 | ADR-0197 (in-place §Decision amendment for X-RateLimit slice per ADR-0052) |
| `x-ratelimit-remaining` | HCM-locally-generated + routed-to-upstream responses (ratelimit filter encode side) | **Set-equal byte-exact** — emission discipline identical to `x-ratelimit-limit`. Value is `<MIN.limit_remaining>` integer-ASCII per the upstream byte format. | Phase 24.2 | ADR-0197 (in-place §Decision amendment for X-RateLimit slice per ADR-0052) |
| `x-ratelimit-reset` | HCM-locally-generated + routed-to-upstream responses (ratelimit filter encode side) | **Set-equal byte-exact** — emission discipline identical to `x-ratelimit-limit`. Value is `<MIN.duration_until_reset.seconds>` integer-ASCII per the upstream byte format. | Phase 24.2 | ADR-0197 (in-place §Decision amendment for X-RateLimit slice per ADR-0052) |

**Phase 24.2 X-RateLimit allow-list extension note:** the three `x-ratelimit-*` rows are set-equal BYTE-EXACT (NOT "permitted-divergence" — the cross-side scenario (g) at fixture `0032-http-ratelimit` asserts byte equality between envoy-go and reference Envoy v1.37.2). The rows are listed in this table so operators consulting the response-header allow-list see the three headers explicitly, but the per-row divergence column ANCHORS `Set-equal byte-exact` rather than presence-only / format-only / value-range. The emission is gated by `enable_x_ratelimit_headers == DRAFT_VERSION_03` (the upstream-default `OFF` arm emits nothing — set-equal vacuously). On the OVER_LIMIT path the headers are injected by the encode hook BEFORE the AMEND-8 [a]+[b]+[c] header order assembly so the X-RateLimit headers are visible to downstream alongside the `x-envoy-ratelimited` + RLS-supplied + filter-config response headers. On the fail-closed error path (`failure_mode_deny=true` + RLS error) the encode hook does NOT run (the `SendLocalReply` constructs the reply without encoder participation per AMEND-8 nullptr-mutate); the headers are NOT emitted; the set-equal contract holds vacuously (both sides omit). MIN-status selection across multi-descriptor responses is by `limit_remaining` with insertion-order tie-break (= descriptor-list order = action-list order per AMEND-6; the FIRST equal-minimum status wins).

---

## Stat-name mapping

The mapping describes, for each emitted stat, the canonical Envoy stat name, the envoy-go internal name (if different), the tag set, and the flows under which values are required to be exact. When a phase introduces a new stat subsystem, it extends this table.

### Flattening rules SN1–SN8 (per ADR-0061; introduced by phase 06.1)

```
Rule SN1: Name segments matching `cluster.<n>.<rest>` extract `<n>` as label
          `envoy_cluster_name` and prefix `<rest>` with `envoy_cluster_`.

Rule SN2: Name segments matching `http.<stat_prefix>.<rest>` extract <stat_prefix>
          as label `envoy_http_conn_manager_prefix` and prefix <rest> with `envoy_http_`.

Rule SN3: Name segments matching `listener.<addr>.<rest>` extract <addr> as label
          `envoy_listener_address` and prefix <rest> with `envoy_listener_`.

Rule SN4: Names ending `_Nxx` where N ∈ {1..5} flatten to a base name with the
          trailing class digit STRIPPED (so the metric name ends in literal `_xx`),
          plus a label `envoy_response_code_class` whose value is the single class
          digit as a string (`"1"`, `"2"`, `"3"`, `"4"`, `"5"`). Empirically verified
          against reference Envoy v1.37.2 at the `ENVOY_TARGET.md`-pinned image
          (server SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee`) on 2026-04-27; see
          empirical evidence block below.

          Examples (canonical):
              cluster.foo.upstream_rq_2xx
                → envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="foo"}
              http.ingress_http.downstream_rq_5xx
                → envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"}

          Counter-examples (NOT what Envoy emits):
              ✗ envoy_cluster_upstream_rq_2xx{...}            -- digit suffix preserved (wrong)
              ✗ ...{envoy_response_code_class="2xx",...}       -- label value with literal "xx" (wrong)
              ✗ envoy_cluster_upstream_rq{envoy_response_code_class="2",...}  -- _xx stripped entirely (wrong)

          Empirical evidence (verbatim excerpt from reference-Envoy /stats/prometheus
          scrape under a 5-request load with statuses [200,200,404,200,500] through
          HCM stat_prefix=ingress_http to cluster c_backend):

              # TYPE envoy_cluster_upstream_rq_xx counter
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c_backend"} 3
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="c_backend"} 1
              envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="c_backend"} 1
              # TYPE envoy_http_downstream_rq_xx counter
              envoy_http_downstream_rq_xx{envoy_response_code_class="1",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_http"} 3
              envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_http"} 0
              envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_http"} 1
              envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"} 1

          Negative-confirmation grep (entire 1181-line scrape, no matches):
              grep -E 'envoy_[a-z_]*_(1xx|2xx|3xx|4xx|5xx)' /stats/prometheus  # -> 0 matches

          Tag-extractor regex source: Envoy v1.37.2
          source/common/config/well_known_names.cc, the `RESPONSE_CODE_CLASS`
          tag entry. Source-tree commit pin = the v1.37.2 release tag, server-side
          version-string SHA `5afe27fb338b16d5bb06b3a7198bcd581b4e3dee` (matches
          ENVOY_TARGET.md). The regex captures the inner `\dxx` token from the
          stat suffix `_<class>xx`, removes the entire `_<class>xx` from the stat
          name (yielding the base name ending `_xx` after the standard rename), and
          emits the captured digit as the `response_code_class` tag value.

Rule SN5: Server-scope names (`server.<rest>`) flatten to `envoy_server_<rest>`
          with no extracted labels.

Rule SN6: HELP text is best-effort English, NOT byte-equal to Envoy's HELP. The
          differential equivalence claim is on values + label keys + types only.

Rule SN7: Histograms are not emitted by 06.1. (Forward-looking.)

Rule SN8: Per-endpoint cluster stats are not emitted by 06.1. (Forward-looking.)
```

### 60-name table (introduced by phase 06.1; extended by phase 09; extended by phase 11; extended by phase 12; UNCHANGED in phase 13; extended by phase 14; extended by phase 15; extended by phase 20; extended by phase 23; extended by phase 24.1 — FIRST cluster-scoped filter-stats surface)

`<stat_prefix>` is read from HCM config (already plumbed from phase 04). `<addr>` is the listener bind address normalized like Envoy does (e.g., `0.0.0.0:10000` → `0.0.0.0_10000`). `<n>` is the cluster name as configured in the bootstrap.

**Listener — 2 names:**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `listener.<addr>.downstream_cx_total` | counter | `envoy_listener_downstream_cx_total{envoy_listener_address="<addr>"}` |
| `listener.<addr>.downstream_cx_active` | gauge | `envoy_listener_downstream_cx_active{envoy_listener_address="<addr>"}` |

**HCM — 5 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `http.<stat_prefix>.downstream_rq_total` | counter | `envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_2xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_3xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_4xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.downstream_rq_5xx` | counter | `envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="<stat_prefix>"}` |

**Cluster — 8 names:**

| Internal name | Type | Prometheus name |
|---|---|---|
| `cluster.<n>.upstream_rq_total` | counter | `envoy_cluster_upstream_rq_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_2xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_3xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="3",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_4xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_rq_5xx` | counter | `envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_total` | counter | `envoy_cluster_upstream_cx_total{envoy_cluster_name="<n>"}` |
| `cluster.<n>.upstream_cx_active` | gauge | `envoy_cluster_upstream_cx_active{envoy_cluster_name="<n>"}` |
| `cluster.<n>.membership_total` | gauge | `envoy_cluster_membership_total{envoy_cluster_name="<n>"}` (Set once at register, equals N endpoints) |

**Server — 2 names (one EMITTED, one explicitly NOT-EMITTED):**

| Internal name | Type | Approximate Prometheus name (verify) |
|---|---|---|
| `server.live` | gauge | `envoy_server_live` (Set to 1 once admin `/ready` returns 200; never reset by 06.1) |
| `server.uptime` | — | **NOT EMITTED** — depends on monotonic-clock + per-scrape recompute; deferred with histograms (see SPEC §2.1) |

**Fault filter — 5 names (introduced by phase 09):**

| Internal name | Type | Prometheus name |
|---|---|---|
| `http.<stat_prefix>.fault.aborts_injected` | counter | `envoy_http_fault_aborts_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.delays_injected` | counter | `envoy_http_fault_delays_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.faults_overflow` | counter | `envoy_http_fault_faults_overflow{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.active_faults` | gauge | `envoy_http_fault_active_faults{envoy_http_conn_manager_prefix="<stat_prefix>"}` |
| `http.<stat_prefix>.fault.response_rl_injected` | counter | `envoy_http_fault_response_rl_injected{envoy_http_conn_manager_prefix="<stat_prefix>"}` |

`response_rl_injected` is emitted as a permanently-zero counter in phase 09
— Envoy emits it even when `response_rate_limit` is not configured (per
phase 09 §11.6 empirical pin); envoy-go matches the surface for differential
parity per ADR-0107 route A. When `response_rate_limit` lands in a future
phase, the same name carries the actual count.

**Local rate-limit filter — 4 names (introduced by phase 11):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<stat_prefix>.http_local_rate_limit.enabled`     | counter | filter | local_ratelimit | every request reaching the filter (§11.5) |
| `<stat_prefix>.http_local_rate_limit.ok`          | counter | filter | local_ratelimit | request not rate-limited (`tryConsume` → true; §11.5) |
| `<stat_prefix>.http_local_rate_limit.rate_limited`| counter | filter | local_ratelimit | request rate-limited (`tryConsume` → false; §11.5) |
| `<stat_prefix>.http_local_rate_limit.enforced`    | counter | filter | local_ratelimit | request rate-limited AND enforced (lockstep with `rate_limited` under MVP per ADR-0118; §11.5) |

**Filter-specific Prometheus tag-extractor (added in phase 11 per ADR-0118):** `<stat_prefix>.http_local_rate_limit.<counter>` extracts the `<stat_prefix>` segment into the Prometheus label `envoy_local_http_ratelimit_prefix`. NOTE: tag-extraction collisions occur if `<stat_prefix>` matches an Envoy-internal tag-extractor name (e.g. `listener` collides with `envoy.listener_address`); the differential fixture 0013 uses safe values (`foo`, `bar`, `baz`, `qux`, `strict`).

**CSRF filter — 3 names (introduced by phase 12):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<stat_prefix>.csrf.request_valid`         | counter | filter | csrf | modifying request whose source origin matches target or `additional_origins[].exact` (§11.6) |
| `http.<stat_prefix>.csrf.request_invalid`       | counter | filter | csrf | modifying request whose source origin is determinable but matches neither (§11.6) |
| `http.<stat_prefix>.csrf.missing_source_origin` | counter | filter | csrf | modifying request whose source origin is undeterminable (§11.6) |

**No new tag-extractor (phase 12):** csrf reuses the existing `envoy_http_conn_manager_prefix` HCM-namespace SN2 extractor — no new pattern needed. UNLIKE phase 11 which added the filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118), phase 12 introduces NO new SN flattening rule.

**Compressor filter — 17 names per HCM stat_prefix (introduced by phase 14):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_compressor_overshadowed`           | counter | filter | compressor | every request where this codec was selectable but overshadowed by a higher-q-value alternative (phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_compressor_used`                   | counter | filter | compressor | every request where this codec was the negotiated selection (regardless of whether response was compressed; phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_identity`                          | counter | filter | compressor | every request where client requested identity (no compression; phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_not_valid`                         | counter | filter | compressor | every request where Accept-Encoding header was malformed (q-value parse error, etc.; phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.header_wildcard`                          | counter | filter | compressor | every request where client sent Accept-Encoding: * (phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.no_accept_header`                         | counter | filter | compressor | every request where client had no Accept-Encoding header (phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.not_compressed_etag`                      | counter | filter | compressor | every response skipped on ETag presence with disable_on_etag_header=true (phase 14 SPEC §11.7) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.compressed`                      | counter | filter | compressor | every response compressed on this codec (phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.content_length_too_small`        | counter | filter | compressor | every response skipped due to body below min_content_length (phase 14 SPEC §11.5 + §11.9) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.not_compressed`                  | counter | filter | compressor | every response skipped (any reason; sum of skip counters; phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.total_compressed_bytes`          | counter | filter | compressor | cumulative compressed-body bytes emitted on response side (phase 14 SPEC §11.5; tolerance per ADR-0133) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.response.total_uncompressed_bytes`        | counter | filter | compressor | cumulative pre-compression body bytes seen on response side (phase 14 SPEC §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.compressed`                       | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP (request_direction_config silent-ignored; phase 14 SPEC §1.1 amendment 1 + §11.5) |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.content_length_too_small`         | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.not_compressed`                   | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.total_compressed_bytes`           | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |
| `<HCM_stat_prefix>.compressor.<library>.<codec>.request.total_uncompressed_bytes`         | counter | filter | compressor | request-side counter; ALWAYS-ZERO in MVP |

Phase 14 adds 17 new rows: 9 active in MVP + 6 always-zero `request.*` (request-side request_direction_config silent-ignored; couples to future request-side compression / decompressor phase) + 2 always-active total_bytes accumulators. NO new SN flattening rule (uses existing SN2 per phase 14 SPEC §1.1 amendment 3). Prometheus rendering: `envoy_http_compressor_<library>_<codec>_<counter>{envoy_http_conn_manager_prefix=<HCM_stat_prefix>}`. `<library_name>` is operator-supplied (`compressor_library.name`); empty allowed; emits with consecutive dots. The `response.` infix appears IFF `response_direction_config` is set on the listener-level Compressor (per compressor.proto line 158-164).

**Bandwidth-limit filter — 14 active names (introduced by phase 15) + 2 deferred histograms (twin-series-filter divergence-window per §1.1 amendment 9):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<stat_prefix>.http_bandwidth_limit.request_enabled`            | counter | filter | bandwidth_limit | stream engaged decode-side throttle (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_enforced`           | counter | filter | bandwidth_limit | per-tick throttle increments; envoy-go bumps by `ticks` at stream-completion to match Envoy's per-fill_interval-tick cumulative semantic (phase 15 SPEC §11.P3 + §6.7) |
| `<stat_prefix>.http_bandwidth_limit.request_incoming_total_size` | counter | filter | bandwidth_limit | cumulative bytes entered decode-side filter (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_allowed_total_size`  | counter | filter | bandwidth_limit | cumulative bytes forwarded through decode-side filter (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_enabled`           | counter | filter | bandwidth_limit | stream engaged encode-side throttle (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_enforced`          | counter | filter | bandwidth_limit | symmetric to request side (phase 15 SPEC §11.P3 + §6.8) |
| `<stat_prefix>.http_bandwidth_limit.response_incoming_total_size` | counter | filter | bandwidth_limit | cumulative bytes entered encode-side filter (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_allowed_total_size`  | counter | filter | bandwidth_limit | cumulative bytes forwarded through encode-side filter (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_pending`            | gauge   | filter | bandwidth_limit | count of streams waiting on decode-side timer (Inc on arm; Dec on fire/cancel; phase 15 SPEC §11.P3 + §11.P13) |
| `<stat_prefix>.http_bandwidth_limit.request_incoming_size`      | gauge   | filter | bandwidth_limit | transient bytes-buffered-but-not-yet-forwarded (decode side; phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.request_allowed_size`       | gauge   | filter | bandwidth_limit | transient bytes-allowed-this-tick (decode side; envoy-go MVP single-blast: set to bodyLen at timer-fire then 0 at OnDestroy; phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_pending`           | gauge   | filter | bandwidth_limit | symmetric to request side (phase 15 SPEC §11.P3 + §11.P13) |
| `<stat_prefix>.http_bandwidth_limit.response_incoming_size`     | gauge   | filter | bandwidth_limit | symmetric to request side (phase 15 SPEC §11.P3) |
| `<stat_prefix>.http_bandwidth_limit.response_allowed_size`      | gauge   | filter | bandwidth_limit | symmetric to request side (phase 15 SPEC §11.P3) |

**Twin-series-filter divergence-window (per phase 15 SPEC §1.1 amendment 9):** 2 unconditional Envoy histograms NOT emitted by envoy-go MVP per phase-06.1 "counters + gauges only" baseline. Differential fixture 0017's `expectations.yaml` allow-lists; the `### Twin-series filter discipline` subsection below extends with the phase-15 entry:

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<stat_prefix>.http_bandwidth_limit.request_transfer_duration`  | histogram | filter | bandwidth_limit | DEFERRED per phase-06.1 baseline; allow-listed via twin-series-filter discipline |
| `<stat_prefix>.http_bandwidth_limit.response_transfer_duration` | histogram | filter | bandwidth_limit | DEFERRED; allow-listed |

Phase 15 adds 14 new active rows (8 counters + 6 gauges) — namespace `<stat_prefix>.http_bandwidth_limit.<counter>` underscore-infix (NOT HCM-rooted per phase 15 SPEC §11.P11). NO new SN flattening rule (the existing `internal/stats/name.go` default-branch flatten handles via dot→underscore substitution; ADR-0061 + ADR-0118 NOT amended). Prometheus rendering: `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` — stat_prefix INLINED into base name; NO labels / NO tag-extractor (per phase 15 SPEC §1.1 amendment 8 + §11.P10).

**RBAC filter — 4 active names (introduced by phase 16):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.allowed`         | counter | filter | rbac | increments per request whose primary engine result = ALLOWED (phase 16 SPEC §11.P6) |
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.denied`          | counter | filter | rbac | increments per request whose primary engine result = DENIED (phase 16 SPEC §11.P6) |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_allowed` | counter | filter | rbac | increments per request whose shadow engine = ALLOWED (when shadow configured; phase 16 SPEC §11.P6) |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_denied`  | counter | filter | rbac | increments per request whose shadow engine = DENIED (when shadow configured; phase 16 SPEC §11.P6) |

**Per-policy counter family (variable; emitted when `track_per_rule_stats: true`):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.policy.<policy_name>.allowed`        | counter | filter | rbac | matched policy under primary ALLOWED |
| `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.policy.<policy_name>.denied`         | counter | filter | rbac | matched policy under primary DENIED |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.policy.<policy_name>.shadow_allowed` | counter | filter | rbac | matched policy under shadow ALLOWED |
| `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.policy.<policy_name>.shadow_denied`  | counter | filter | rbac | matched policy under shadow DENIED |

NOTE: Per-policy counter cost is operator-config-driven; the table entries are templates, not fixed names. The `.policy.` infix segment was empirically refined at Task 8 reference-Envoy v1.37.2 scrape per ADR-0145 (REFINES phase 16 SPEC §13.2 stub which omitted the segment). Operators with N policies × 2 base sides × 2 (primary + shadow) emit up to 4N per-policy counters per active namespace. Foot-gun documented at `### Phase 16 forward-pointer notes`.

Phase 16 adds 4 new active rows (4 counters) — namespace `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule per phase 16 SPEC §1.1 amendment 9 + ADR-0145 — SN2-reuse hypothesis RATIFIED at Task 8 empirical scrape. Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_rbac_<rules_stat_prefix>_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`.

**jwt_authn filter — 7 names (introduced by phase 17; 5 actively-emitted under MVP + 2 STRUCTURALLY UNREACHABLE):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.jwt_authn.allowed`                 | counter | filter | jwt_authn | increments per request whose active-engine result = ALLOWED (phase 17 SPEC §3) |
| `http.<HCM_stat_prefix>.jwt_authn.denied`                  | counter | filter | jwt_authn | increments per request denied by JWT validation (phase 17 SPEC §3) |
| `http.<HCM_stat_prefix>.jwt_authn.cors_preflight_bypassed` | counter | filter | jwt_authn | increments per OPTIONS preflight bypassed via `bypass_cors_preflight` (phase 17 §1.1 amendment 10) |
| `http.<HCM_stat_prefix>.jwt_authn.jwks_fetch_success`      | counter | filter | jwt_authn | increments once per RemoteJwks provider whose initial blocking JWKS fetch succeeded at filter-load time (phase 17 ADR-0154 §Decision (vii)) |
| `http.<HCM_stat_prefix>.jwt_authn.jwks_fetch_failed`       | counter | filter | jwt_authn | increments per failed JWKS `Get()` at request time |
| `http.<HCM_stat_prefix>.jwt_authn.jwt_cache_hit`           | counter | filter | jwt_authn | STRUCTURALLY UNREACHABLE under MVP — `jwt_cache_config` silent-ignored per §8 deferral 8; registered but never incremented (publishes 0) |
| `http.<HCM_stat_prefix>.jwt_authn.jwt_cache_miss`          | counter | filter | jwt_authn | STRUCTURALLY UNREACHABLE under MVP — registered but never incremented (publishes 0) |

Phase 17 adds 7 new rows (7 counters; 5 active + 2 structurally-unreachable) — namespace `http.<HCM_stat_prefix>.jwt_authn.<counter>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule per phase 17 SPEC §1.1 amendment 9 + §11.P7 + ADR-0154 — SN2-reuse hypothesis RATIFIED at the Task 13 fixture-0019 empirical scrape (both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form). Per-route stats are SHARED with listener-level per ADR-0153 + ADR-0154 (the 8th canonical per-route is pure string-reference-delegation; spawns no new policy-evaluation state; mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats discipline). NO per-provider scaling per §1.1 amendment 9 (multiple providers contribute to the same filter-wide counter set). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`.

**ext_authz filter — 6 names (introduced by phase 18.1; 5 actively-emitted under MVP + 1 STRUCTURALLY UNREACHABLE):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.ext_authz.ok`                     | counter | filter | ext_authz | increments per request whose auth-check result = ALLOWED (HTTP 200 from auth service; phase 18.1 ADR-0163) |
| `http.<HCM_stat_prefix>.ext_authz.denied`                 | counter | filter | ext_authz | increments per request denied by the auth service (recognized deny status 401/403; phase 18.1 ADR-0163) |
| `http.<HCM_stat_prefix>.ext_authz.error`                  | counter | filter | ext_authz | increments per auth-check error (connect failure / timeout / unrecognized status; phase 18.1 ADR-0163) |
| `http.<HCM_stat_prefix>.ext_authz.disabled`               | counter | filter | ext_authz | STRUCTURALLY UNREACHABLE under MVP — `filter_enabled` silent-ignored per parent §5.P12 + §6 amendment 7; registered but never incremented (publishes 0; couples to deferred Runtime `filter_enabled` gate) |
| `http.<HCM_stat_prefix>.ext_authz.failure_mode_allowed`   | counter | filter | ext_authz | increments per request where an auth error was bypassed via `failure_mode_allow:true` (phase 18.1 ADR-0163; also increments `error` simultaneously) |
| `http.<HCM_stat_prefix>.ext_authz.invalid`                | counter | filter | ext_authz | increments per header-mutation rejected by `validate_mutations` gating (phase 18.1 ADR-0161 + ADR-0163) |

Phase 18.1 adds 6 new rows (6 counters; 5 active + 1 structurally-unreachable) — namespace `http.<HCM_stat_prefix>.ext_authz.<counter>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac, phase-17 jwt_authn; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule per ADR-0163 — SN2-reuse hypothesis RATIFIED at Task 8 empirical scrape (both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form). Per-route stats SHARED with listener-level per ADR-0163 (5th-canonical-REUSE; spawns no new policy-evaluation state). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`.

**ext_proc filter — 9 names (introduced by phase 19.1):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.ext_proc.streams_started`                       | counter | filter | ext_proc | increments at first-stage dispatch per stream (gRPC mode: openProcessorStream entry; HTTP mode: first per-stage POST). Phase 19.1 ADR-0167 + ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.stream_msgs_sent`                      | counter | filter | ext_proc | increments per ProcessingRequest sent on the stream (gRPC mode: every stream.Send; HTTP mode: every per-stage POST). Phase 19.1 ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.stream_msgs_received`                  | counter | filter | ext_proc | increments per ProcessingResponse received on the stream (gRPC mode: every successful stream.Recv; HTTP mode: every 2xx response body). Phase 19.1 ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.spurious_msgs_received`                | counter | filter | ext_proc | increments per response classified as spurious — `disable_immediate_response:true` + ImmediateResponse arrival; mutation_rules-rejected header mutation (once per stage); allowed_override_modes allowlist miss; more-than-once-per-stage override_message_timeout. Phase 19.1 ADR-0171 + ADR-0172 + ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.streams_failed`                        | counter | filter | ext_proc | increments per transport-level error (gRPC connect/timeout/ctx.Err; HTTP non-2xx; codec unmarshal error). Phase 19.1 ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.streams_closed`                        | counter | filter | ext_proc | increments on clean stream termination via OnDestroy (sync.Once-guarded; CloseSend best-effort). Phase 19.1 ADR-0171 |
| `http.<HCM_stat_prefix>.ext_proc.failure_mode_allowed`                  | counter | filter | ext_proc | increments per stream where a transport error was bypassed via `failure_mode_allow:true` (also increments `streams_failed` simultaneously). STRUCTURALLY UNREACHABLE when `failure_mode_allow:false` (the proto default). Phase 19.1 ADR-0173 |
| `http.<HCM_stat_prefix>.ext_proc.override_message_timeout_received`     | counter | filter | ext_proc | increments per ProcessingResponse.override_message_timeout that was accepted (in-range + at-most-once-per-stage + max_message_timeout ≥ 1ms). Phase 19.1 ADR-0171 |
| `http.<HCM_stat_prefix>.ext_proc.override_message_timeout_ignored`      | counter | filter | ext_proc | increments per override_message_timeout that was REJECTED — out-of-range OR more-than-once-per-stage OR max_message_timeout=0 (override API disabled). STRUCTURALLY UNREACHABLE when max_message_timeout=0 AND processor never sends override. Phase 19.1 ADR-0171 |

Phase 19.1 adds 9 new rows (9 counters; 7 unconditionally-active + 2 STRUCTURALLY-UNREACHABLE-under-default-MVP-config) — namespace `http.<HCM_stat_prefix>.ext_proc.<counter>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac, phase-17 jwt_authn, phase-18.1 ext_authz; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule per ADR-0173 — SN2-reuse hypothesis RATIFIED at the Task 13 fixture-0022 empirical scrape (the 9 hypothesized counters all emitted by reference Envoy v1.37.2 + an additional 8 reference-only counters DEFERRED to phase 19.2+ activation per §19.P4 RATIFIED-WITH-AMENDMENT). Per-route stats SHARED with listener-level per ADR-0173 (5th-canonical-REUSE; SECOND CONSECUTIVE §9 row to REUSE the 5th canonical after phase 18.1 ext_authz). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_ext_proc_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`. The Task 13 fixture-harness assertion gate is relaxed to "counter NAMES present on BOTH sides" (not cross-side delta match) to accommodate the partial-roster MVP per ADR-0173 §Consequences.

**oauth2 filter — 6 names (introduced by phase 20):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.oauth2.oauth_unauthorized_rq` | counter | filter | oauth2 | increments per category-(d) 401 emission on the bad-state-cookie path (per phase 20 SPEC §4.2 + §4.6 + ADR-0181) |
| `http.<HCM_stat_prefix>.oauth2.oauth_failure`         | counter | filter | oauth2 | increments per category-(d) 401 emission on the token_endpoint terminal-4xx path (NOT also on the refresh-failure path; refresh-failure → category-(a) 302 per §4.7 + ADR-0183) |
| `http.<HCM_stat_prefix>.oauth2.oauth_passthrough`     | counter | filter | oauth2 | increments per `pass_through_matcher` hit — the request bypasses oauth2 entirely (per phase 20 SPEC §4.6 + ADR-0181) |
| `http.<HCM_stat_prefix>.oauth2.oauth_success`         | counter | filter | oauth2 | increments per category-(b) 302 post-callback-success emission — the auth-code flow's successful token_endpoint POST + envelope rollout (per phase 20 SPEC §4.6 + ADR-0181) |
| `http.<HCM_stat_prefix>.oauth2.oauth_refreshtoken_success` | counter | filter | oauth2 | increments per successful silent refresh-token rotation — the deferred Set-Cookie envelope rides the upstream response (per phase 20 SPEC §4.6 + ADR-0183) |
| `http.<HCM_stat_prefix>.oauth2.oauth_refreshtoken_failure` | counter | filter | oauth2 | increments per failed silent refresh-token rotation — the request then falls through to category-(a) 302 challenge (per phase 20 SPEC §4.6 + ADR-0183) |

Phase 20 adds 6 new rows (6 counters; all UNCONDITIONALLY active under default MVP config — NONE structurally unreachable) — namespace `http.<HCM_stat_prefix>.oauth2.<counter>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac, phase-17 jwt_authn, phase-18.1+18.2 ext_authz, phase-19.1+19.2 ext_proc; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule per ADR-0181 + AMEND-4 — SN2-reuse hypothesis RATIFIED at the phase-20 SPEC §20.P8 REFUTED scrape (the 6 wire-exact upstream counters all emitted by reference Envoy v1.37.2; the BRAINSTORM-hypothesized 8th + 9th counters `signout_completed` + `cookie_decrypt_failure` REFUTED as RATIFIED-AS-ABSENT per §20.P11 + AMEND-4). **NO per-route counter qualifier per ADR-0180 + §5 REUSE-by-absence** (the v1.37.x oauth2 proto has no `OAuth2PerRoute` message; THIRD CONSECUTIVE §9 row to skip per-route surface — STRONGER form than phase-18 + phase-19 5th-canonical REUSE). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_oauth2_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`. All 6 counters registered UNCONDITIONALLY at `New()` time per the phase-17/18/19 STRUCTURALLY-UNREACHABLE-counter unconditional-registration discipline (no MVP-vs-non-MVP gating; the default config exercises every counter path).

**Total: 86 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + **0 from 18.2** + **9 from 19.1** + **0 from 19.2** — the 6 ext_authz counters are mode-agnostic; phase 19.2 holds at 0 net new ext_proc counters per the phase-19.2 SPEC §2 item 5 + §13 item 2 — body-mode AMENDMENT activates additional EMITS on the existing 9-counter roster (`stream_msgs_sent` / `stream_msgs_received` increment on each body-stage outbound/inbound; `spurious_msgs_received` increments on body-stage `streamed_response` PARSE-REJECT) but introduces NO new counter names; the 8 reference-only counters from §19.P4 RATIFIED-WITH-AMENDMENT stay DEFERRED to phase 19.3+ activation). The four `downstream_rq_Nxx` and four `upstream_rq_Nxx` Prometheus exposition forms collapse to two base-name groups (one HCM, one cluster) per the Rule SN4 status-class flattening discipline. The 2 deferred histograms (phase 15) + the per-policy counter family (phase 16; operator-config-driven) are documented separately; they do NOT count in the 86-name base total. Of phase 17's 7 names, 5 actively emit under MVP and 2 (`jwt_cache_hit` / `jwt_cache_miss`) are STRUCTURALLY UNREACHABLE. Of phase 18.1's 6 names, 5 actively emit under MVP and 1 (`disabled`) is STRUCTURALLY UNREACHABLE. Of phase 19.1's 9 names, 7 actively emit under default MVP config and 2 (`failure_mode_allowed`, `override_message_timeout_ignored`) are STRUCTURALLY UNREACHABLE under default config (gated on `failure_mode_allow:true` and `max_message_timeout >= 1ms` respectively). Phase 18.2 lands ZERO new counters — the gRPC-mode `mapGRPCResponse` increments the same 5-active + 1-unreachable counter set as 18.1's HTTP-mode `mapHTTPResponse`. The reference-only 8 ext_proc counters from the §19.P4 AMENDMENT (`immediate_responses_sent`, `message_timeouts`, `clear_route_cache_disabled`, `clear_route_cache_ignored`, `clear_route_cache_upstream_ignored`, `rejected_header_mutations`, `server_half_closed`, `http_not_ok_resp_received`) are tracked as 19.2+ activation surfaces; they do NOT count in the 86-name MVP base total.

**Phase 20 extension — 86 → 92 internal names:** phase 20 adds 6 new oauth2 counter rows (per AMEND-4 + ADR-0181 + §11 §20.P8 REFUTED — the BRAINSTORM-hypothesized 8th + 9th counters `signout_completed` + `cookie_decrypt_failure` REFUTED as RATIFIED-AS-ABSENT per §20.P11). All 6 actively emit under default MVP config (NONE structurally unreachable — DIVERGES from the phase-17 jwt_authn / phase-18.1 ext_authz / phase-19.1 ext_proc precedent of carrying STRUCTURALLY-UNREACHABLE counter rows). Phase 20 total: **86 → 92 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + **6 from phase 20**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the pre-phase-20 convention.

**adaptive_concurrency filter — 7 names (introduced by phase 21; 1 counter + 6 gauges):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.rq_blocked` | counter | filter | adaptive_concurrency | increments per Block emission from `forwardingDecision()` CAS-loop overflow (per phase 21 SPEC §6.4 + AMEND-6 + ADR-0186) |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.concurrency_limit` | gauge | filter | adaptive_concurrency | int64 raw uint32 — current per-HCM-instance limit (per phase 21 SPEC §6.6 + ADR-0186) |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.gradient` | gauge | filter | adaptive_concurrency | int64 ×1000 per ADR-0059 §Decision AMENDMENT — bounded [500, 2000] over the gradient's [0.5, 2.0] domain |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.burst_queue_size` | gauge | filter | adaptive_concurrency | int64 signed — per-tick burst-headroom value (sqrt of current-limit × gradient; per SPEC §4.4 semantics correction from BRAINSTORM §5.1) |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.sample_rtt_msecs` | gauge | filter | adaptive_concurrency | int64 nanoseconds per AMEND-3 C3 + ADR-0059 §Decision AMENDMENT — envoy-go-strict departure from upstream's milliseconds; stat NAME preserves byte-exact upstream `sample_rtt_msecs` |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.min_rtt_msecs` | gauge | filter | adaptive_concurrency | int64 nanoseconds per AMEND-3 C3 + ADR-0059 §Decision AMENDMENT — envoy-go-strict departure; stat NAME preserves byte-exact upstream `min_rtt_msecs` |
| `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.min_rtt_calculation_active` | gauge | filter | adaptive_concurrency | int64 0/1 via `stats.BoolToInt` per ADR-0059 §Decision AMENDMENT — 1 during the minRTT recalc window per SPEC §4.5 + AMEND-2 C4 first-tick |

**lua filter — 3 names (introduced by phase 22.1; 3 counters) + 5 names (added by phase 22.2; 5 counters) = 8 names total post-22.2:**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.errors` | counter | filter | lua | increments per `*lua.ApiError` / Lua runtime error / panic-recovery wrapper trip per script invocation (per phase 22.1 SPEC §8 + parent §7 + AMEND-3 — upstream-parity per `lua_filter.cc` `ALL_LUA_FILTER_STATS` macro). |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.executions` | counter | filter | lua | increments per script invocation (`envoy_on_request` / `envoy_on_response` call). **Upstream-parity per AMEND-3** + `lua_filter.cc:872` (`stats_.executions_.inc()`); REFUTES BRAINSTORM §5.1 + §5.4 envoy-go-strict classification. |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.respond_calls` | counter | filter | lua | increments per `request_handle:respond()` short-circuit invocation. **envoy-go-strict extension per AMEND-3** (NOT in upstream surface; collision-free verified at parent SPEC §11.5.3). Operator-visibility rationale: knowing the `:respond()` short-circuit rate enables dashboards + alerting for auth-deny / rate-limit-deny / geo-block surfaces. |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.httpcall_total` | counter | filter | lua | **envoy-go-strict extension per phase 22.2 SPEC §7.1 + ADR-0192.** Increments per `:httpCall(cluster, headers, body, timeout_ms, async?)` invocation regardless of sync vs async dispatch arm. NOT in upstream surface (upstream Envoy v1.37.2 emits only `errors` + `executions` per `ALL_LUA_FILTER_STATS` macro). Operator-visibility rationale: outbound-call rate per stat_prefix enables dashboards for cluster-side fan-out from operator scripts (auth-side-channel rate, geo-IP lookup rate, etc.). |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.httpcall_failures` | counter | filter | lua | **envoy-go-strict extension per phase 22.2 SPEC §7.1 + ADR-0192 + AMEND-22.2-3 D6 closure.** Increments per SYNC `:httpCall` failure (transport error OR retry-exhausted 5xx per `httpclient.Options.RetryPolicy.RetryOnStatus`). **SYNC-ONLY** — async fire-and-forget dispatches per AMEND-22.2-3 D6 use a `noopCallbacks` discard channel; their failures are invisible at filter-stats per upstream parity. |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.httpcall_timeouts` | counter | filter | lua | **envoy-go-strict extension per phase 22.2 SPEC §7.1 + ADR-0192 + AMEND-22.2-3 D6 closure.** Increments per SYNC `:httpCall` timeout (`context.DeadlineExceeded` from `httpclient.Client.ClusterDispatch` retry-loop budget exhaustion). **SYNC-ONLY** — same async-invisibility discipline as `httpcall_failures`. |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.body_buffered_bytes_total` | counter | filter | lua | **envoy-go-strict extension per phase 22.2 SPEC §7.1 + ADR-0192.** Cumulative bytes accumulated in the per-stream `decodedBodyBytes` / `encodedBodyBytes` slices across all streams that invoke `:body()` or `:bodyChunks()`. Increment site: the defensive-copy moment at endStream per §11.3 D3 (`atomic.AddUint64`-style accumulation across streams). Operator-visibility rationale: body-buffer capacity planning + early-warning signal for operator scripts that buffer multi-MB bodies in steady state. |
| `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.coroutine_yields_total` | counter | filter | lua | **envoy-go-strict extension per phase 22.2 SPEC §7.1 + ADR-0192.** Cumulative coroutine yield events from `:body()` (1 yield per script invocation that calls `:body()` before endStream) + `:bodyChunks()` + sync `:httpCall()` (1 yield per sync dispatch). Operator-visibility rationale: yield-heavy scripts are inefficient body-streaming patterns; perf debugging signal. |

**Phase 22.2 extension — 102 → 107 internal names:** phase 22.2 adds 5 new lua-filter counter rows (5 envoy-go-strict counters per ADR-0192) under the SAME namespace template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` HCM-rooted per AMEND-2 (UNCHANGED from 22.1 — no new SN flattening rule; SN2-reuse RATIFIED at phase-22.1 SPEC §8 carries forward). All 5 unconditionally active under default MVP config (continues the phase-20/21/22.1 pattern of NONE structurally unreachable). NO per-route counter qualifier UNCHANGED at 22.2 (arm-18 PARSE-REJECT still active; per-route deferred to 22.3). Prometheus rendering via existing SN2 extractor: `envoy_http_lua_<config_stat_prefix>_<stat>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}` (same shape as 22.1 — 22.2 just adds 5 names under the existing template). Phase 22.2 total: **102 → 107 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + 7 from 21 + 3 from 22.1 + **5 from phase 22.2**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the pre-phase-22.2 convention. **All 5 phase-22.2 counters are envoy-go-strict** (upstream Envoy v1.37.2's `ALL_LUA_FILTER_STATS` macro emits ONLY `errors` + `executions`); 22.2 IMPL lands 5 NEW envoy-go-strict departure records under `### envoy.filters.http.lua` 22.2 sub-section. **`httpcall_failures` + `httpcall_timeouts` are SYNC-ONLY per AMEND-22.2-3 D6 closure** (async fire-and-forget invisible at filter-stats per upstream `noopCallbacks` discipline — operator scripts that need failure visibility must use sync dispatch). The optional 6th counter `dynmd_writes_total` (dynamic-metadata write count) was DEFERRED at 22.2 SPEC §7.1 per BRAINSTORM Q11 pragmatic-middle — omitted unless 22.2 IMPL surfaces operator-value signal. 22.2 IMPL surfaced no such signal; the 5-counter bundle STANDS at 22.2 phase-done.

**admission_control filter — 3 names (introduced by phase 23; 3 counters):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `http.<HCM_stat_prefix>.admission_control.rq_rejected` | counter | filter | admission_control | increments per request rejected by the probabilistic admit/reject decision (per phase 23 SPEC §6.5 + ADR-0194) |
| `http.<HCM_stat_prefix>.admission_control.rq_success` | counter | filter | admission_control | increments per admitted request classified as an upstream success at encode time (per phase 23 SPEC §6.6 + ADR-0194) |
| `http.<HCM_stat_prefix>.admission_control.rq_failure` | counter | filter | admission_control | increments per admitted request classified as an upstream failure at encode time (**NOT** `rq_error` — name byte-exact-upstream per AMEND-3 + `ALL_ADMISSION_CONTROL_STATS(COUNTER)` macro at `admission_control.h:35-38`) |

Phase 23 adds 3 new rows (3 counters; all UNCONDITIONALLY active under default config — NONE structurally unreachable) — namespace `http.<HCM_stat_prefix>.admission_control.<stat>` HCM-rooted (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18.1+18.2 ext_authz / phase-19.1+19.2 ext_proc / phase-20 oauth2 / phase-21 adaptive_concurrency / phase-22 lua; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule; SN2-reuse RATIFIED at phase-23 SPEC §11 D1. All 3 counters are registered UNCONDITIONALLY at `New()` time per the established filter-stats unconditional-registration discipline. Per-route stats are SHARED with listener-level (vacuously — the v1.37.2 proto has no `AdmissionControlPerRoute` message; REUSE-by-absence per phase-23 SPEC §5.4 + ADR-0195). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_admission_control_<stat>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`.

**ratelimit filter — 4 names (introduced by phase 24.1; 4 counters; FIRST CLUSTER-SCOPED filter-stats surface):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].ok` | counter | filter | ratelimit | increments per `ShouldRateLimit` response with `overall_code = OK` (admit-the-request) per parent SPEC §4.7 + AMEND-1 + ADR-0197[core] |
| `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].error` | counter | filter | ratelimit | increments per `ShouldRateLimit` gRPC call returning a transport/timeout/cancel/`UNKNOWN` error per parent SPEC §4.7 + AMEND-1; ALWAYS Inc on the error arm (additively with `failure_mode_allowed` on the fail-open path) |
| `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].over_limit` | counter | filter | ratelimit | increments per `ShouldRateLimit` response with `overall_code = OVER_LIMIT` (reject-the-request) per parent SPEC §4.7 + AMEND-1 |
| `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].failure_mode_allowed` | counter | filter | ratelimit | increments per error arm AND `failure_mode_deny=false` (fail-OPEN admit) per parent SPEC §4.7 + AMEND-1; incremented ALONGSIDE `error` on the error-fail-open path; NOT incremented on the error-fail-closed path |

Phase 24.1 adds 4 new rows (4 counters; all UNCONDITIONALLY active under default config — NONE structurally unreachable; the `failure_mode_allowed` counter increments only on the error-fail-open path but the row is unconditionally REGISTERED at `New()` time per the established filter-stats unconditional-registration discipline). **Namespace `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` CLUSTER-ROOTED** (NOT HCM-rooted) per AMEND-1 — DIVERGES from every prior §9 family-row's HCM-rooted shape AND from phase-15 bandwidth_limit's non-HCM-rooted shape (which is `cluster.<HCM_stat_prefix>.http_bandwidth_limit.<stat>`). This is the **FIRST cluster-scoped cross-namespace filter-stats surface** — LANDS the pattern that ext_authz's `charge_cluster_response_stats` DEFERRED per AMEND-10. The `<rls_cluster_name>` is the upstream RLS cluster name captured at HCM-parse-time from `rate_limit_service.grpc_service.envoy_grpc.cluster_name`; the OPTIONAL `<stat_prefix>` segment (the 24.1 `compiledConfig.statPrefix` AMEND-3 13th field) is elided WHOLESALE (including its leading dot) when empty. Registration uses `(*stats.Registry).NewCounterIfAbsent` (the POST-Freeze-safe idempotent path per ADR-0117) — load-bearing across MULTIPLE listeners sharing one RLS cluster (each filter-instance gets the SAME `*Counter` handle for each leaf name; charges from all instances aggregate into the same atomic cell). NO new SN flattening rule; SN1-reuse RATIFIED at phase-24.1 SPEC §6 (CLUSTER-rooted via the existing `envoy_cluster_name` SN1 extractor + dot→underscore default-branch flatten). Per-route stats: SHARED with listener-level at 24.1 (vacuously — `RateLimitPerRoute.domain` override LANDS at 24.2 with the NEW 10th canonical per ADR-0125 §(xv) amendment). Prometheus rendering via existing SN1 extractor: when `stat_prefix` empty, `envoy_cluster_ratelimit_<stat>{envoy_cluster_name="<rls_cluster_name>"}`; when `stat_prefix` non-empty, `envoy_cluster_ratelimit_<stat_prefix>_<stat>{envoy_cluster_name="<rls_cluster_name>"}`.

**Phase 23 extension — 107 → 110 internal names:** phase 23 adds 3 new admission_control-filter counter rows (3 upstream-parity counters per ADR-0194 + AMEND-3) under the namespace template `http.<HCM_stat_prefix>.admission_control.<stat>` HCM-rooted. All 3 unconditionally active (continues the phase-20/21/22 pattern of NONE structurally unreachable). NO per-route counter qualifier (REUSE-by-absence — no `AdmissionControlPerRoute` proto surface; see admission_control subsection below). Phase 23 total: **107 → 110 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + 7 from 21 + 3 from 22.1 + 5 from 22.2 + 0 from 22.3 + **3 from phase 23**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the long-standing convention. **All 3 phase-23 counters are upstream-parity** (byte-exact names from the upstream `ALL_ADMISSION_CONTROL_STATS(COUNTER)` macro at `admission_control.h:35-38`; confirmed at SPEC §11 D1 empirical scrape of reference Envoy v1.37.2); 0 envoy-go-strict departure records in the stat table at phase 23 (the SINGLE departure record — RTDS `runtime_key` PARSE-REJECT — is in the admission_control subsection above, not in the stat table).

**Phase 24.1 extension — 110 → 114 internal names:** phase 24.1 adds 4 new ratelimit-filter counter rows (4 upstream-parity counters per AMEND-1 + ADR-0197[core]) under the namespace template `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` **CLUSTER-rooted** (DIVERGES from every phase-09..23 §9 family-row's HCM-rooted shape — this is the **FIRST cluster-scoped cross-namespace filter-stats surface**, LANDS the pattern that ext_authz's `charge_cluster_response_stats` DEFERRED per AMEND-10). All 4 unconditionally REGISTERED (continues the phase-20/21/22/23 pattern of NONE structurally unreachable at registration time; the `failure_mode_allowed` counter increments only on the error-fail-open path but the row is unconditionally registered at `New()` time). NO per-route counter qualifier at 24.1 (the `RateLimitPerRoute.domain` override LANDS at 24.2 with the NEW 10th canonical per ADR-0125 §(xv) amendment; the per-route domain may extend the qualifier surface at 24.2 SPEC). Phase 24.1 total: **110 → 114 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + 7 from 21 + 3 from 22.1 + 5 from 22.2 + 0 from 22.3 + 3 from 23 + **4 from phase 24.1**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the long-standing convention. **All 4 phase-24.1 counters are upstream-parity** (byte-exact names from the upstream ratelimit-filter `cluster.<rls>.ratelimit[.<stat_prefix>].{ok,error,over_limit,failure_mode_allowed}` namespace; confirmed at parent SPEC §11 D2 empirical scrape of reference Envoy v1.37.2 per AMEND-1); 0 envoy-go-strict departure records in the stat table at phase 24.1 (the 3 phase-24.1 departure records — `disable_key` PARSE-REJECT + `extension` action PARSE-REJECT + `dynamic_metadata` action PARSE-REJECT — are in the ratelimit subsection above, not in the stat table).

**Phase 24.2 per-route `domain`-qualifier disposition — 114 stays 114 (per AMEND-1):** the `RateLimitPerRoute.domain` override (the per-route `domain` field that takes precedence over the filter-config `domain` when set; landed at 24.2 Task 3 + 4 with the 10th canonical per ADR-0199) is a **descriptor-tier override, NOT a stat namespace**. When a per-route `domain` is set, the descriptor sent to the RLS gRPC call carries the per-route `domain` value (overriding the filter-config `domain`); the 4-counter stat namespace `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` remains UNCHANGED — the counter names + the `<rls_cluster_name>` + the optional `<stat_prefix>` segment are all derived from the filter-config (the RLS cluster name + the filter-config `stat_prefix`); none of these incorporate the per-route `domain`. **Stat count 110 → 114 STAYS at 114 after phase 24.2; no new rows, no per-route qualifier extension.** Confirmed at parent SPEC §11 D2 + AMEND-1 — the upstream ratelimit-filter stat shape exposes `cluster.<rls>.ratelimit[.<stat_prefix>].<stat>` ONLY; the per-route `domain` is descriptor-tier (visible to the RLS service via the `RateLimitRequest.domain` field) and does NOT participate in the stat naming surface. Per-route stats SHARED with listener-level remains the discipline at 24.2 (same shape as 24.1; the per-route `domain` does NOT split the counter series — operators wishing to observe per-domain rate-limit decision aggregates do so via the RLS service's own observability surface, NOT via the envoy-go filter's stat surface). 0 stat-shape divergences from 24.1 + 0 new envoy-go-strict departures at phase 24.2 (the 3 phase-24.1 departure records cover the surface; `override_option` accepted-but-INERT is upstream-parity, NOT a departure).

**wasm filter — 5 names (introduced by phase 25.1; 4 counters + 1 gauge; tri-group prefix structure per AMEND-A2 — DIVERGES from the dominant HCM-rooted §9 family-row pattern):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `wasm.wazero.created` | counter | filter | wasm | increments per per-stream `*wasm.VM` construction at `internal/filter/http/wasm/decode_headers.go` per phase 25.1 SPEC §4.2 + parent §7 + AMEND-A2. **Upstream-parity** Group-B (`wasm.<runtime>.*`); `<runtime>` is uniformly `"wazero"` at 25.1 (all 3 alternative discriminators `v8`/`wamr`/`wasmtime` PARSE-REJECT per parent §6.2 arm 11 + AMEND-A2). Idempotent registration via `NewCounterIfAbsent` (ADR-0117) so multiple plugin configs on the same listener share the single Group-B counter. |
| `wasm.wazero.active` | gauge | filter | wasm | live `*wasm.VM` count; increments on construct + decrements on `vm.Close()` at OnDestroy. **Upstream-parity** Group-B per AMEND-A2; idempotent `NewGaugeIfAbsent` for the same listener-sharing rationale. |
| `wasm.<plugin_name>.executions` | counter | filter | wasm | increments per `proxy_on_request_headers` dispatch (DECODE-SIDE ONLY per the Task 15+17 follow-up — encode-side Inc was the bug; SPEC §4.2 + §5.1 hostcall 1 + AMEND-A2 pin decode-only). **envoy-go-strict extension per AMEND-A2** (NOT in upstream surface; consolidated departure record below). `<plugin_name>` is `PluginConfig.name` Group-C discriminator. |
| `wasm.<plugin_name>.hostcall_denied` | counter | filter | wasm | increments per default-deny capability sandbox denial at `internal/filter/http/wasm/abi_callbacks.go` (anchors the AMEND-A5 default-deny posture + ADR-0204). **envoy-go-strict extension per AMEND-A2 + AMEND-A5.** Counter increments are paired with `WasmResult::InternalFailure` (=10) return-to-guest + integration error log (`slog.Error("Attempted call to restricted proxy-wasm capability: <name>")`) per ADR-0204. |
| `wasm.<plugin_name>.envoy_go.failures` | counter | filter | wasm | increments per VM-failure event (`vm.runtime` panic-wrapper trip / wazero trap that escapes the per-callback CallProxyOnX boundary). **envoy-go-strict extension per AMEND-A2** — single observable counter replacing upstream's `FailState`-via-event surface; the upstream `wasm.<plugin_name>.vm_reload_*` Group-C counter family (a 3-counter triplet keyed by reload-cause) is DEFERRED to 25.3 per parent §7.4. The dotted-suffix `envoy_go.failures` form is INTENTIONAL: the internal `envoy_go.` segment marks the envoy-go-strict origin of the metric (visible at `/stats/prometheus` via the wasm-flattening rule landed at the Task 15+17 follow-up — `envoy_wasm_<plugin>_envoy_go_failures`). At 25.2 the `envoy_go.failures` scope EXTENDS to also increment on body-buffer cap exceeded + shared-data cap exceeded events (per §2.25 + AMEND-B3). |

**wasm filter — 9 NEW envoy-go-strict counter rows added at phase 25.2 (per §7.1 + AMEND-B3; project stat surface 119 → 128):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `wasm.<plugin_name>.tick_invocations` | counter | filter | wasm | increments per `proxy_on_tick(rootCtxID)` invocation (per-RootVM tick goroutine; 10ms envoy-go-strict floor per Q5). **envoy-go-strict extension per 25.2 §7.1.** Operator visibility into tick dispatch rate; tick-period clamping (period < 10ms → 10ms) means this counter under-reports a guest-intended sub-10ms rate. |
| `wasm.<plugin_name>.http_call_dispatched` | counter | filter | wasm | increments per `proxy_http_call` invocation that successfully dispatches to an upstream cluster (cluster lookup OK; AsyncClient request started). **envoy-go-strict extension per 25.2 §7.1 + AMEND-B3.** Pairs with `wasm.<plugin>.http_call_response`; healthy dispatch pattern keeps the two counters in ratio. |
| `wasm.<plugin_name>.http_call_response` | counter | filter | wasm | increments per `proxy_on_http_call_response(streamCtxID, call_token, ...)` invocation (response routed to a live stream context). **envoy-go-strict extension per 25.2 §7.1.** Guest-side observability into outbound HTTP dispatch reply rate. |
| `wasm.<plugin_name>.foreign_function_denied` | counter | filter | wasm | increments per `proxy_call_foreign_function` invocation that returns `WasmResult::NotFound` (=1) — typically the EMPTY default registry path per AMEND-A9. **envoy-go-strict extension per 25.2 §7.1 + AMEND-A9.** Operator visibility into foreign-function call attempts against unregistered names; non-zero with no explicit `wasm.RegisterForeignFunction` calls signals a guest expecting upstream-default foreign functions (10 in upstream; 0 in envoy-go-strict). |
| `wasm.<plugin_name>.body_buffer_cap_exceeded` | counter | filter | wasm | increments when accumulated body buffer exceeds `envoy_go_strict_body_buffer_cap_bytes` (default 16 MiB); stream closes with 413 (decode side) or response terminates (encode side); `wasm.<plugin>.envoy_go.failures` ALSO increments per §2.25 scope extension. **envoy-go-strict extension per 25.2 §7.1 + Q2.** |
| `wasm.<plugin_name>.http_call_dispatch_unknown_cluster` | counter | filter | wasm | increments per `proxy_http_call` to unknown cluster (returns `BadArgument` per upstream + AMEND-B3). **envoy-go-strict extension per 25.2 §7.1 + Q4.** |
| `wasm.<plugin_name>.shared_data_cap_exceeded` | counter | filter | wasm | increments when `proxy_set_shared_data` exceeds value cap (1 MiB default per Q6) OR entry-count cap (1024 default); returns `WasmResult::InternalFailure`; `wasm.<plugin>.envoy_go.failures` ALSO increments per §2.25. **envoy-go-strict extension per 25.2 §7.1 + Q6.** |
| `wasm.<plugin_name>.dynamic_stats_cap_exceeded` | counter | filter | wasm | increments when `proxy_define_metric` exceeds dynamic-stats entry cap (1024 default per Q9); the define call returns `ErrCapExceeded` → `WasmResult::InternalFailure`. **envoy-go-strict extension per 25.2 §7.1 + Q9.** |
| `wasm.<plugin_name>.http_call_response_after_close` | counter | filter | wasm | increments when an outbound HTTP call's response arrives AFTER the originating stream context has been closed (defensive observability for the cancel-at-destruction race per AMEND-B3; near-zero in healthy operation; non-zero pages an operator that envoy-go's cancellation path has a bug). **envoy-go-strict extension per 25.2 §7.1 + AMEND-B3 (RAISES BRAINSTORM Q9 8 → 9).** |

**wasm filter — 4 NEW counter rows added at phase 25.3 (Group-C `vm_reload_*` triplet upstream-parity + 1 envoy-go-strict `env_vars_cap_exceeded`; project stat surface 128 → 132):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `wasm.<plugin_name>.vm_reload_success` | counter | filter | wasm | increments when a Failed FAIL_RELOAD VM is reinstantiated into a fresh un-poisoned instance past the backoff window (request-driven recovery). **Upstream-parity Group-C counter** (mirrors upstream `VmReloadSuccess`); activated at 25.3 with the `failure_policy = FAIL_RELOAD` reload state machine per ADR-0211. |
| `wasm.<plugin_name>.vm_reload_runtime_failure` | counter | filter | wasm | increments per FAILED reload ATTEMPT (the reinstantiate itself errors). **Upstream-parity Group-C counter** (mirrors upstream `VmReloadFailure`). NOTE: fires ONLY on a failed reload attempt — NOT on the trap-arming or the recover-success path (a successful recover increments `vm_reload_success`, not this). |
| `wasm.<plugin_name>.vm_reload_backoff` | counter | filter | wasm | increments when a reload is requested while still within the backoff window (`base_interval = max(operator, 100ms)` floor) — the reload is rate-limited, not attempted. **Upstream-parity Group-C counter** (mirrors upstream `VmReloadBackoff`). |
| `wasm.<plugin_name>.env_vars_cap_exceeded` | counter | filter | wasm | ALLOCATED at the env_vars cap PARSE-REJECT (64 entries / 4096 bytes per value). **envoy-go-strict extension per 25.3 + ADR-0211.** **Allocate-only at boot-PARSE-REJECT:** the cap fires at config-load (`parseEnvVars`) where there is no running per-plugin stats scope to increment it; the counter is allocated on the stat surface (so the surface is 132) but is NOT incremented at config-load — consistent with the other 25.2 cap counters being runtime-only. It exists on the stat surface for future runtime use. |

**Open-ended dynamic-stats family `wasmcustom.<custom_name>`** (per AMEND-B2; NOT counted in static stat name total). Operator-extensible at runtime via `proxy_define_metric`; capped at 1024 entries envoy-go-strict (cap-exceeded → `wasm.<plugin>.dynamic_stats_cap_exceeded` counter + `WasmResult::InternalFailure`). Per-plugin isolation via per-plugin `*dynamic.Registry` SCOPE — each `*compiledConfig` constructs its own Registry rooted at `stats.RootScope.Subscope("wasm").Subscope(pluginName)`; admin /stats enumerates as `wasm.<plugin_name>.wasmcustom.<custom_name>` (parent-scope-rooted) but the in-wire stat name (from the proxy-wasm wire perspective) is `wasmcustom.<custom_name>` byte-faithful to upstream. NEW `internal/stats/dynamic/` infrastructure subpackage anchors the Registry primitive per ADR-0208 + AMEND-B2.

**Phase 25.2 extension — 119 → 128 internal names:** phase 25.2 adds 9 new envoy-go-strict counters (5 at filter scope per §7.1 base + 4 cap/event counters; consolidated per AMEND-B3 to 9). Phase 25.2 total: **119 → 128 internal names** (114 from pre-25.1 + 5 from phase 25.1 + **9 from phase 25.2**). All 9 are envoy-go-strict (NOT in upstream surface; consolidated into the 25.2 departure record #1 + #4 below per the AMEND-B3 + Q9 stat-roster bundle).

**Phase 25.3 extension — 128 → 132 internal names:** phase 25.3 ACTIVATES the upstream `vm_reload_*` Group-C 3-counter triplet (`vm_reload_success` / `vm_reload_runtime_failure` / `vm_reload_backoff` — upstream-parity, NOT envoy-go-strict) with the `failure_policy = FAIL_RELOAD` reload state machine per ADR-0211, PLUS 1 envoy-go-strict counter `env_vars_cap_exceeded` (allocate-only at boot-PARSE-REJECT — see the precise behavioral note in its stat-table row above + the 25.3 EXTENSION block below). Phase 25.3 total: **128 → 132 internal names** (128 from pre-25.3 + 4 from phase 25.3). This is the FAMILY-FINAL stat count for the §9 HTTP-filters family (which closed at phase 25.3 phase-done); the NEXT stat-surface delta is the phase-26.3 `rbac_network` 4-counter roster (§9 Network-filters family).

**rbac_network filter — 4 NEW counter rows added at phase 26.3 (`envoy.filters.network.rbac` L4 filter — enforced + shadow result counters; project stat surface 132 → 136):**

| Internal name | Type | Source | Filter | Description |
|---|---|---|---|---|
| `<stat_prefix>.rbac.allowed` | counter | filter | rbac_network | increments when the ENFORCED engine returns ALLOWED for a connection (the L4 OnData decision). **Upstream-parity** (mirrors upstream `RBAC` filter's `allowed` counter). Rooted on the PGV-required `stat_prefix` (network shape — NOT the HCM-rooted `http.<HCM>.rbac.*` HTTP shape). |
| `<stat_prefix>.rbac.denied` | counter | filter | rbac_network | increments when the ENFORCED engine returns DENIED (→ `rbac_deny_close` response-code-details + `Close(NoFlush)`). **Upstream-parity** (mirrors upstream `denied`). |
| `<stat_prefix>.rbac.[<shadow_rules_stat_prefix>.]shadow_allowed` | counter | filter | rbac_network | increments when the SHADOW engine returns ALLOWED (never affects the enforced disposition). **Upstream-parity** (mirrors upstream `shadow_allowed`). When `shadow_rules_stat_prefix` is set it inserts a segment between `rbac.` and `shadow_allowed` (SPEC §7.1); the enforced counters are unaffected. Publishes 0 when no shadow engine is configured (predeclared-empty for scrape stability). |
| `<stat_prefix>.rbac.[<shadow_rules_stat_prefix>.]shadow_denied` | counter | filter | rbac_network | increments when the SHADOW engine returns DENIED. **Upstream-parity** (mirrors upstream `shadow_denied`). |

**Phase 26.3 extension — 132 → 136 internal names:** phase 26.3 adds the 4 `rbac_network` base counters (`allowed` / `denied` / `shadow_allowed` / `shadow_denied` — ALL upstream-parity, NONE envoy-go-strict). All four are unconditionally predeclared at config-load (register-at-parse via `NewCounterIfAbsent`, idempotent) so they exist at scrape even before any traffic (Prometheus best practice; continues the phase-20/21/22/23 NONE-structurally-unreachable pattern). The network RBAC proto carries NO `track_per_rule_stats` field, so `rbac_network` emits ONLY the 4 static base counters — the engine's per-policy `PerPolicyCounters` machinery (the phase-16 HTTP-rbac per-policy lazy `sync.Map` family) stays DORMANT for consumer #2 (F2; D-26.3-7 + D-P5). Phase 26.3 total: **132 → 136 internal names** (132 from pre-26.3 + 4 from phase 26.3). Prometheus rendering via the NEW network-rbac tag-extractor at `internal/stats/name.go` (the `.rbac.` arm; see below).

**Phase 29.1 extension — 337 → 360 internal names:** phase 29.1 adds the **23** `mongo_proxy` fixed stats (`envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` — the §9 Network-filters family's FOURTH row + second stats-PRIMARY filter) — **22 counters + the `op_query_active` GAUGE** (the project's first network-filter gauge in the differential surface; its inc/dec increments land at 29.2, but the row is CREATED at 29.1). The roster is created EAGERLY at config-load under scope `mongo.<stat_prefix>.` — a byte-mirror of upstream's `ALL_MONGO_PROXY_STATS` macro (parent SPEC AMEND-B1/B3/B4, live-probed + source-pinned against reference Envoy v1.37.2; `delays_injected` is PLURAL — the regression guard). Per-row enumeration of 23 rows is NOT reproduced here (the 25.x / 28.1 large-roster convention); the roster is normatively defined by `internal/filter/network/mongoproxy/stats.go::rosterSuffixes()` + the R2 golden test `TestStatRoster_MatchesUpstreamMacro`. The 22 counters: `cx_destroy_local_with_active_rq`, `cx_destroy_remote_with_active_rq`, `cx_drain_close`, `decoding_error`, `delays_injected`, `op_command`, `op_command_reply`, `op_get_more`, `op_insert`, `op_kill_cursors`, `op_query`, `op_query_await_data`, `op_query_exhaust`, `op_query_multi_get`, `op_query_no_cursor_timeout`, `op_query_no_max_time`, `op_query_scatter_get`, `op_query_tailable_cursor`, `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor` + the `op_query_active` gauge = 23. Request-side counters increment at 29.1; the response-side `op_reply*`/`op_command_reply` + the gauge increments wire at 29.2; `cx_drain_close` + `delays_injected` at 29.3 — all 23 exist-at-zero from config-load (creation parity, D-P1). The dynamic `cmd.<cmd>.total` / `collection.<c>.query.*` / `collection.<c>.callsite.<cs>.query.*` families are config/traffic-dependent and are NOT counted in the static 360 surface (the zookeeper `auth.<scheme>_rq` precedent). All 23 are upstream-parity (none envoy-go-strict); the envoy-go-strict departures are behavioral (the D-P1 boot-window creation posture; the `stats.IsValidName` guard on un-nameable wire-derived dynamic segments — both recorded as coverage boundaries in the mongo_proxy subsection above, not as stat-table departure rows). Phase 29.1 total: **337 → 360 internal names** (337 from pre-28.2 [= pre-29.1; phase 28.2 held at 337] + **23 from phase 29.1**). Prometheus rendering is TAG-EXTRACTED (NOT flat — the INVERSE of phase-28 zookeeper) via the NEW `mongo.` four-rule tag-extractor arm at `internal/stats/name.go` (`envoy_mongo_<leaf>{envoy_mongo_prefix="<sp>"[,envoy_mongo_cmd|collection|callsite=…]}`; AMEND-B2 + AMEND-C1; see the mongo_proxy subsection + the `mongo.` arm note below). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters continue NOT counted in the base total per the long-standing convention.

**Phase 28.2 extension — 337 → 337 internal names (zero creation delta):** phase 28.2 wires the response-side increments of the 201 `zookeeper_proxy` counters created at 28.1 — all `*_resp` / `*_resp_bytes` / `*_resp_fast` / `*_resp_slow` / `response_bytes` / `watch_event` counters are now increment-active (subject to the `enable_*` flag gates). Zero new counters are created. Fixture `0048-zookeeper-responses` is the cross-side proof (R5 ratification); the 38th fuzzer covers the response path. The stat total stays **337**.

**Phase 28.1 extension — 136 → 337 internal names:** phase 28.1 adds the **201** `zookeeper_proxy` counters (`envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` — the §9 Network-filters family's first stats-PRIMARY filter), the largest single-filter addition in the project. The counters form the EAGER roster created at config-load under scope `<stat_prefix>.zookeeper.` — a byte-mirror of upstream's `ALL_ZOOKEEPER_PROXY_STATS(COUNTER)` macro (parent SPEC AMEND-A1/A2, live-probed against reference Envoy v1.37.2; the prefix order is `<stat_prefix>` FIRST). Per-row enumeration of 201 rows is NOT reproduced here (the 25.x large-roster precedent); the roster is normatively defined by `internal/filter/network/zookeeperproxy/stats.go::rosterSuffixes()` + the R2 golden-list unit test. The 201 families: **4 plain** (`decoder_error`, `request_bytes`, `response_bytes`, `watch_event`) + **28 `<op>_rq`** + **29 `<op>_rq_bytes`** + **28 `<op>_decoder_error`** + **28×4** response-side (`<op>_resp` / `<op>_resp_bytes` / `<op>_resp_fast` / `<op>_resp_slow`) = 201. The macro asymmetries are mirrored EXACTLY (AMEND-A1/A2/A3): `connect_readonly_rq` + `connect_readonly_rq_bytes` are rq-side-only (NO `connect_readonly_resp`); there is NO static `auth_rq` counter — auth requests increment LAZY dynamic per-scheme counters `<stat_prefix>.zookeeper.auth.<scheme>_rq` instead (a non-builtin scheme collapses to `auth.unknown_scheme_rq`, upstream `getBuiltin`-fallback parity); `auth_resp*` ARE present in the macro/roster; the `_rq_bytes` family carries 29 names (the extra `auth_rq_bytes` + `connect_readonly_rq_bytes` + `setauth_rq_bytes` vs the 28-name `_rq` family). The four `enable_*` flags gate INCREMENTS, NEVER creation — a flag-false counter exists at 0 forever; the response-side counters (`_resp*`) likewise exist-at-zero until 28.2 wires the response decoder (creation parity, D-P5). All 201 are upstream-parity (none envoy-go-strict). Phase 28.1 total: **136 → 337 internal names** (132 from pre-26.3 + 4 from phase 26.3 + 0 from phase 27 + **201 from phase 28.1**). The roll lands at **28.1b** (not 28.1a): the BEHAVIOR_CONTRACT records cross-side-PROVEN surface, and the proof is the now-green `0046-zookeeper-requests` cross-side `StatsAsserter` fixture (the deliberate 28.1a deferral). Prometheus rendering is FLAT (no labels) via the NEW `.zookeeper.` inline-prefix arm at `internal/stats/name.go` (see the zookeeper_proxy subsection + the `.zookeeper.` arm note below; AMEND-A4). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters continue NOT counted in the base total per the long-standing convention.

**Phase 26.3 network-rbac Prometheus tag-extractor (NEW production observability — per ADR-0218 §Consequences, the prom-projection rule).** The `rbac_network` counters are NOT HCM-rooted (the HTTP rbac filter's stats flow through the SN2 `http.<hcm>.rbac.*` path and never reach this arm), so they need a dedicated `internal/stats/name.go::flattenToProm` arm. The detection is the literal `.rbac.` segment with a `stat_prefix` head (idx > 0). The head before `.rbac.` is promoted to the `envoy_rbac_prefix` label; the remainder (`rbac.[<shadow_prefix>.]<counter>`) flattens to base `envoy_rbac_<rest>` (dots→underscores; SN4 status-class collapse is SKIPPED — rbac names have no `_Nxx` suffix). Reference-parity examples: `rbac_allow.rbac.allowed → envoy_rbac_allowed{envoy_rbac_prefix="rbac_allow"}`; `rbac_deny.rbac.denied → envoy_rbac_denied{envoy_rbac_prefix="rbac_deny"}`; `rbac_shadow.rbac.shadow_ns.shadow_denied → envoy_rbac_shadow_ns_shadow_denied{envoy_rbac_prefix="rbac_shadow"}`. This is the SECOND non-HCM-rooted §9 prom-projection arm (joins the phase-25.1 `wasm.` arm + the phase-15 `bandwidth_limit` precedent for non-HCM-rooted filter-stat shapes). The arm is keyed to the four known network-rbac counter suffixes (the optional `shadow_rules_stat_prefix` sits between `rbac.` and the two `shadow_*` counters) — adding any 5th network-rbac counter requires extending the suffix set in lockstep with `newFilterStats` registration.

Phase 25.1 adds 5 new rows (4 counters + 1 gauge) — namespace tri-group prefix structure per AMEND-A2 (DIVERGES from the dominant HCM-rooted §9 family-row pattern preserved at fault/csrf/compressor/rbac/jwt_authn/ext_authz/ext_proc/oauth2/adaptive_concurrency/lua/admission_control; this is the FIRST §9 family-row to use a non-HCM-rooted top-level scope prefix combined with a per-runtime + per-plugin twin-axis split). The structural divergence is upstream-parity preservation, NOT envoy-go-strict (upstream `source/extensions/filters/http/wasm/config.h:51-53` drops the HCM-injected `stats_prefix` segment; the wasm filter's stat surface is rooted at `wasm.<runtime>.*` and `wasm.<plugin_name>.*` directly). All 5 unconditionally REGISTERED at filter `New()` time (continues the phase-20/21/22/23 pattern of NONE structurally unreachable). NEW SN9 flattening rule landed at `internal/stats/name.go::flattenToProm` at the Task 15+17 follow-up: `case strings.HasPrefix(internal, "wasm.")` arm — NO label promotion; `<scope>` INLINED into base; internal dots in `<rest>` converted to `_`. Projection: `wasm.wazero.created` → `envoy_wasm_wazero_created`; `wasm.plugin_e.executions` → `envoy_wasm_plugin_e_executions`; `wasm.plugin_e.envoy_go.failures` → `envoy_wasm_plugin_e_envoy_go_failures`. NO per-route counter qualifier at 25.1 (the 5th-canonical REUSE-by-absence per AMEND-A3; per-route lands at 25.3 IF the SPEC empirical scrape surfaces a `WasmPerRoute` proto with novel shape — current AMEND-A3 confirms ABSENCE-DEFINITIVE — the 5-counter surface STAYS unchanged at 25.3 since per-route stats are SHARED with listener-level). Prometheus rendering via the NEW `wasm.` arm flattening rule landed at this 25.1 phase-done bundle.

**Phase 25.1 extension — 114 → 119 internal names:** phase 25.1 adds 5 new wasm-filter rows (4 counters + 1 gauge per ADR-0203 + AMEND-A2). Phase 25.1 total: **114 → 119 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + 7 from 21 + 3 from 22.1 + 5 from 22.2 + 0 from 22.3 + 3 from 23 + 4 from 24.1 + 0 from 24.2 + **5 from phase 25.1**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the long-standing convention. **2 of 5 phase-25.1 counters are upstream-parity** (`wasm.wazero.created` + `wasm.wazero.active`; mirror the upstream `source/extensions/common/wasm/stats.h` `WasmRuntimeStats` shape — Group-B in AMEND-A2 terminology). **3 of 5 are envoy-go-strict** (`wasm.<plugin_name>.executions` + `wasm.<plugin_name>.hostcall_denied` + `wasm.<plugin_name>.envoy_go.failures`); these are consolidated into the SINGLE departure record #3 below per the AMEND-A2 stat-roster bundle (rather than 3 separate records per the 22.2 lua precedent — wasm's 3 counters are jointly-introduced + jointly-anchored at AMEND-A2 + ADR-0203 §Decision). The upstream `vm_reload_*` Group-C 3-counter triplet (`vm_reload_success` / `vm_reload_runtime_failure` / `vm_reload_backoff`) is DEFERRED to 25.3 per parent §7.4 forward-pointer (depends on the `failure_policy = FAIL_RELOAD` reload disposition lifted at 25.3); STAYS at 119 at 25.2 phase-done (25.2 adds ~4 envoy-go-strict counters per the 25.2 BRAINSTORM scope hand-off below — projected total 119 → ~123 at 25.2 phase-done). The optional 6th counter `bytes_loaded_total` (cumulative wasm-bytecode bytes loaded across all configurations) was DEFERRED at 25.1 SPEC §8 — omitted unless operator-value signal surfaces at 25.2.

**Phase 22.1 extension — 99 → 102 internal names:** phase 22.1 adds 3 new lua-filter rows (3 counters per AMEND-3 + ADR-0189) — namespace `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` HCM-rooted per AMEND-2 (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18.1+18.2 ext_authz / phase-19.1+19.2 ext_proc / phase-20 oauth2 / phase-21 adaptive_concurrency; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule; SN2-reuse RATIFIED at phase-22.1 SPEC §8 (HCM-rooted via the existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten). All 3 unconditionally active under default MVP config (continues the phase-20/21 pattern of NONE structurally unreachable — DIVERGES from the phase-17 jwt_authn / phase-18.1 ext_authz / phase-19.1 ext_proc precedent). NO per-route counter qualifier per arm-18 PARSE-REJECT (per-route deferred to 22.3; the NEW 9th canonical per ADR-0125 §(xiv) AMENDMENT may extend the surface at 22.3 IMPL — 22.3 SPEC settles whether per-route counters are SHARED-vacuous or split). Prometheus rendering via existing SN2 extractor: `envoy_http_lua_<config_stat_prefix>_<stat>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`. Phase 22.1 total: **99 → 102 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + 7 from 21 + **3 from phase 22.1**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the pre-phase-22.1 convention. **`respond_calls` is the SOLE envoy-go-strict counter at phase 22.1** (was 2 in BRAINSTORM §5.4; AMEND-3 correctly reclassified `executions` to upstream-parity via the upstream-scrape evidence; the corrected bundle drops from 2 records to 1 record per parent §1.1 AMEND-3 + §14 edit #4).

**Phase 21 extension — 92 → 99 internal names:** phase 21 adds 7 new adaptive_concurrency rows (1 counter + 6 gauges per AMEND-3 + ADR-0186) — namespace `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` HCM-rooted (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18.1+18.2 ext_authz / phase-19.1+19.2 ext_proc / phase-20 oauth2; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule; SN2-reuse RATIFIED at phase-21 SPEC §11 §21.P3 PARTIAL. All 7 unconditionally active under default MVP config (continues the phase-20 pattern of NONE structurally unreachable — DIVERGES from the phase-17 jwt_authn / phase-18.1 ext_authz / phase-19.1 ext_proc precedent). NO per-route counter qualifier per REUSE-by-absence (FOURTH CONSECUTIVE §9 row to skip ADR-0125 amendment). Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor + dot→underscore default-branch flatten: `envoy_http_adaptive_concurrency_gradient_controller_<stat>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`. Phase 21 total: **92 → 99 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16 + 7 from 17 + 6 from 18.1 + 0 from 18.2 + 9 from 19.1 + 0 from 19.2 + 6 from 20 + **7 from phase 21**). The phase-15 deferred histograms + the phase-16 per-policy counter family + the 8 reference-only ext_proc counters from §19.P4 RATIFIED-WITH-AMENDMENT continue NOT counted in the base total per the pre-phase-21 convention.

**ADR-0059 §Decision AMENDMENT cross-reference (per phase 21 AMEND-7):** the float-valued-gauge int64 encoding convention added at phase 21 codifies three value-classes layered atop the unchanged `*stats.Gauge` int64 primitive: (1) time-typed gauges as int64 nanoseconds direct via `Gauge.Set(d.Nanoseconds())` — envoy-go-strict departure from upstream's milliseconds; stat NAMES preserve byte-exact upstream (`sample_rtt_msecs`, `min_rtt_msecs`); (2) ratio-typed gauges as int64 ×1000 via `Gauge.Set(int64(ratio * 1000))` — matches upstream's integer-millis convention; gives 3 decimal places of precision over the bounded `[0.5, 2.0]` domain (the `gradient` gauge); (3) bool-typed gauges as int64 0/1 via `Gauge.Set(stats.BoolToInt(b))` — NEW `internal/stats/conv.go` `BoolToInt(b bool) int64` sibling helper (~10 LoC; cross-phase reusable). The unchanged `*stats.Gauge` primitive is the only structural delta; the convention is layered at each callsite (gradient + sampleRTTMsecs + minRTTMsecs + minRTTCalculationActive) and documented at each metric's `# HELP` text per Rule SN6 best-effort English. Cross-references: ADR-0186 (paired phase-21 ADR; consumer); ADR-0059 §Decision AMENDMENT body (DECISIONS.md line ~2109); SPEC §3.2 + AMEND-7.

**Phase 13 (buffer filter) note:** The `envoy.filters.http.buffer` filter shipped in phase 13 contributes ZERO new entries to this table. The filter has no filter-specific counter namespace at all (confirmed empirically at phase 13 SPEC §11.5 — reference Envoy v1.37.2 emits NO `envoy_http_buffer_*` counter family). Buffer-filter overflow is observable on the envoy-go side via the existing `downstream_rq_4xx` HCM counter (rendered via Rule SN4 status-class flattening as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`). The Envoy-only HCM counters `downstream_rq_too_large` (increments on every 413) and `downstream_rq_completed` (increments on every completed request) are NOT in this table; they are filtered out of the differential per the `### Twin-series filter discipline` allow-list discipline below.

### Twin-series filter discipline (per empirical-verification scrape)

> **Twin-series filter discipline (per empirical-verification scrape):** Envoy v1.37.2 ALSO emits two twin metric families that envoy-go does NOT emit and the differential fixture (§7) MUST filter out before per-counter delta comparison: (a) `envoy_cluster_external_upstream_rq_xx` (the "external" upstream-rq twin Envoy uses to split internal vs external traffic via `internal_traffic` config); (b) `envoy_listener_http_downstream_rq_xx` (a listener-scoped HCM-rq twin keyed by both listener address and HCM stat_prefix); plus the per-exact-status family `envoy_cluster_upstream_rq{envoy_response_code="200"}` (a separate metric family with `envoy_response_code` label, distinct from `envoy_cluster_upstream_rq_xx`'s `envoy_response_code_class` label). The fixture's allow-list enumerates exactly the 13 unique Prometheus names this SPEC ships; everything else in the Envoy scrape is ignored.

> **Phase 09 fault-stat route-A note (forward-pointer per ADR-0107).** Phase 09 takes route A for `fault.response_rl_injected` (emit a permanently-zero counter rather than omit the line) — the twin-series-discipline analog but with envoy-go-side emission. Reference Envoy v1.37.2 emits `envoy_http_fault_response_rl_injected{...}` even when `response_rate_limit` is unconfigured; envoy-go matches the surface so the differential allow-list does not have to per-line-skip. The 22-name table above reflects the route A choice. When `response_rate_limit` lands in a future phase, the same registered counter starts incrementing without any framework change.

> **Phase 15 bandwidth_limit twin-series histogram extension (per ADR-0138 §Decision (vi) + phase 15 SPEC §1.1 amendment 9 + §13.4):** Envoy v1.37.2 emits 2 UNCONDITIONAL transfer-duration histograms per active stat_prefix (`<prefix>.http_bandwidth_limit.request_transfer_duration` + `<prefix>.http_bandwidth_limit.response_transfer_duration`) — fire regardless of `enable_response_trailers` setting. envoy-go MVP per phase-06.1 "counters + gauges only" baseline emits NO histograms. Differential fixture 0017's `expectations.yaml` allow-lists both via this twin-series-filter discipline. Prometheus surface seen by operators: `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_bucket{}` + `_sum` + `_count` series on Envoy side, ABSENT on envoy-go side. Future re-activation: histogram-emit-infra phase lands `*stats.Registry.Histogram` + Prometheus `histogram_*` extractor; `filterStats` extends with 2 histogram fields keyed by `stat_prefix`.

---

## Access log field mapping

*Introduced by phase 06.2. Justified by ADR-0066 (architecture: in-tree file sink + AsyncFileSink + drop-newest backpressure), ADR-0067 (reject log_format at parse), ADR-0068 (this subsection — three-tier per-record per-field equivalence matrix), ADR-0069 (server.accesslog_dropped counter naming).*

The access-log field mapping enumerates every operator in the Envoy default
access-log format (15 operators in identical positions on every record) per
ADR-0066, the per-operator equivalence tier per ADR-0068's three-tier matrix,
and the empirical-pin block recording the verbatim format-string shape from
reference Envoy v1.37.2. The differential equivalence claim (the
"Semantically equal after field-mapping" predicate from the Equivalence
Matrix row at line 18) IS the three-tier matrix below.

### 15-operator default format (per ADR-0066; empirical-pin in §11 of the 06.2 SPEC)

[<START_TIME>] "<:METHOD> <:PATH> <PROTOCOL>" <RESPONSE_CODE> <RESPONSE_FLAGS>
<BYTES_RECEIVED> <BYTES_SENT> <DURATION> <RESP-SVC-TIME> "<X-FORWARDED-FOR>"
"<USER-AGENT>" "<X-REQUEST-ID>" "<:AUTHORITY>" "<UPSTREAM_HOST>"

### Three-tier matrix (per ADR-0068)

Tier E (exact byte-equal cross-side; 7 operators):
  :METHOD, :PATH, PROTOCOL, RESPONSE_CODE, BYTES_SENT, USER-AGENT, :AUTHORITY

Tier F (format-only — parses to expected shape on both sides; cross-side value
not asserted equal; 3 operators):
  START_TIME (RFC3339 ms-precision UTC, within workload wall-clock window)
  DURATION (int ms ≥ 0)
  UPSTREAM_HOST (`<host>:<port>` for routed; `-` for direct_response)

Tier S (subject must emit `-`; reference unconstrained; 5 operators):
  RESPONSE_FLAGS, BYTES_RECEIVED, RESP(X-ENVOY-UPSTREAM-SERVICE-TIME),
  X-FORWARDED-FOR, X-REQUEST-ID

Counts: 7 + 3 + 5 = 15 (= the operator count in the format).

### X-ENVOY-ORIGINAL-PATH?:PATH fallback note (per 06.2 SPEC §6.1)

Operator #3 in the format is %REQ(X-ENVOY-ORIGINAL-PATH?:PATH)% — emit the
original-path header if present, else fall through to :PATH. Neither side
emits X-ENVOY-ORIGINAL-PATH on fixture 0006's workload (envoy-go does not
inject it; reference Envoy doesn't either, because fixture 0006 has no
path_rewrite-bearing route); both sides emit :PATH via the fallback. A
future phase introducing path-rewriting must re-evaluate fixture 0006's
Tier-E/F expectations under the new behavior.

### Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log)

```
Empirical evidence (verbatim excerpt from reference-Envoy /tmp/envoy-access.log
under the 5-request workload from §7.2; reference image v1.37.2 at
ENVOY_TARGET.md SHA c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd;
captured 2026-04-30 by phase 06.2 PLAN Task 3 step 3):

[2026-04-30T09:10:30.856Z] "GET /health HTTP/1.1" 200 - 0 3 0 - "-" "curl/8.5.0" "b66c2c7d-3921-4184-b6c1-6a80dd5e7e8e" "127.0.0.1:15006" "-"
[2026-04-30T09:10:30.861Z] "GET /api/v1/foo HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "1210434d-5aa4-4a56-a256-3ff6fc989ce5" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.865Z] "GET /api/v1/bar HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "c76bd1e7-3f55-4a6b-a3df-f88f00c7250a" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.870Z] "GET /api/v1/baz HTTP/1.1" 200 - 0 15 0 0 "-" "curl/8.5.0" "5b25ba00-2be4-4ae6-9693-0ce90609f529" "127.0.0.1:15006" "192.168.65.2:18443"
[2026-04-30T09:10:30.875Z] "GET /notfound HTTP/1.1" 404 - 0 10 0 - "-" "curl/8.5.0" "5a9c562a-1ebf-4676-a556-bf02f89a0fad" "127.0.0.1:15006" "-"
```

The block above is paste-verbatim-synchronized with `docs/envoy-go/phases/06.2-access-log/SPEC.md` §11 lines 567-578 (mirrors the 06.1 BEHAVIOR_CONTRACT `## Stat-name mapping` Rule SN4 paste-verbatim discipline; the two blocks must change atomically — any future Envoy image bump per `ENVOY_TARGET.md`'s refresh procedure that alters the format requires updating both locations in the same commit, no drift permitted).

### Applies to

- Phase-06.2 envoy-go `internal/accesslog` package + the four HCM emit-deferral sites (`directResponseAction.do`, `routerAction.do`, `h2DirectResponseAdapter.WriteH2`, `routerActionH2.doH2`), exercised via fixture `0006-access-log` (H1 differential) + `internal/filter/hcm/accesslog_emit_test.go` (H2 unit tests).
- The 15-operator Envoy default format only. Custom format strings (the `log_format`/`format_string`/`json_format` typed-config fields) are rejected at parse-time per ADR-0067.

### Does not yet apply to

- Operators not plumbed in 06.2 (5 of 15: RESPONSE_FLAGS, BYTES_RECEIVED, RESP-SVC-TIME, X-FORWARDED-FOR, X-REQUEST-ID — Tier S subject-emits-`-`).
- Sinks other than `envoy.access_loggers.file` (stdout / tcp_grpc (gRPC ALS) / open_telemetry — silently-ignored per ADR-0041 06.2 amendment).
- Per-route access-log filters (`access_log[].filter` — silently-ignored).
- Log rotation, fsync, durability ceilings (out of scope per SPEC §2.1).
- Trailers in access logs (deferred to gRPC family per ADR-0058).
- Access-log records for ctx-cancelled requests (skipped per the H2 zero-status sentinel, SPEC §2.1).
- SIGTERM-while-record-pending drain semantics (Phase 08's deliverable).

---

## xDS wire state machine

_to be filled per-phase as needed._

This subsection captures the ADS/delta state machine as the contract expects: allowed message orderings, ACK/NACK semantics, version/nonce rules, resource-subscription handling, reconnection behavior, and initial-fetch-timeout semantics. It is extended as xDS-family phases land.

---

## Timing tolerances

_to be filled per-phase as needed._

Timing is not compared by default. A phase may opt in to latency bounds (p50 / p95 / p99 vs upstream, wall-clock or CPU-normalized) by adding a subsection here naming the fixture, the bound, and the measurement methodology. Opt-ins are additive; removing one requires a superseding ADR.

- **Fault filter delay accuracy: ±10ms (per phase 09 §11.2 empirical pin).**
  envoy-go's `time.AfterFunc` timer-driven async-resume matches Envoy v1.37.2's
  fault delay accuracy within ±10ms across the 50/100/200/500ms sweep.
  Empirical worst-case overhead observed: +3.6ms (Envoy v1.37.2 was tested;
  envoy-go's overhead is similar). The differential fixture 0011-http-fault's
  driver bucketizes elapsed timings (fast vs delayed) to absorb CI scheduling
  jitter while still distinguishing the wholesale-override no-delay path from
  the inherited-delay path.

- **Local rate-limit token-bucket refill boundary: ±10ms (per phase 11 ADR-0116 + SPEC §11.7 empirical pin).**
  fixture 0013 scenario 3 (refill-after-fill_interval): t=250ms refill boundary ±10ms wall-clock. Per ADR-0116 + §11.7 empirical (BRAINSTORM ±20ms hypothesis narrowed; PLAN author may widen back to ±20ms with retry-with-deadline harness if CI flakes per SPEC §12 D4).

- **Bandwidth-limit throttle wall-clock tolerance: ±70ms per-side (per phase 15 ADR-0137 + SPEC §7.3 + §11.P9 + Task-14 per-side discipline adoption).**
  Phase 15's Path B-async body algorithm emits one body-blast at the end of the throttle window; Envoy's reference Path A rate-paced chunk-emit pattern emits chunks at `fill_interval` cadence with `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes per tick (KiB/s units per phase 15 SPEC §1.1 amendment 6). Total wall-clock throttle-time observably equivalent within ±70ms across body sizes 100 bytes to 51 KiB and `limit_kbps` values 1 to 100 (per phase 15 SPEC §11 empirical test matrix at probes A/B/L). **Task-14 empirical pin:** the SPEC §11.P9(c) "cross-side total-throttle-time converges within ±70ms" predicate was REFUTED for bodies within initial-burst capacity (Envoy's initial-burst-discount + per-request bump-on-active-side-regardless-of-body diverges from envoy-go's deterministic ceil-formula + DecodeData-driven bump). Adopted discipline: **each side asserted independently within ±70ms of its predicted target** (NOT cross-side wall-clock delta). Fixture 0017's driver asserts per-scenario per-side wall-clock within tolerance; the ±70ms window absorbs `time.AfterFunc` Linux granularity (typically 1-5ms minimum) plus CI scheduling jitter plus initial-burst-capacity approximation variance.

---

## Admin API

The envoy-go admin server is a single HTTP/1.1 plaintext bind allocated by `internal/admin.Server.Start()` (per phase 01 contract; reused unchanged in 06.1 and 08.1). Seven endpoints are registered on the same `*http.ServeMux`: `/ready` (phase 01), `/stats/prometheus` (phase 06.1), `/config_dump`, `/clusters`, `/listeners`, `/server_info` (phase 08.1), `POST /drain_listeners` (phase 08.2; NEW mutating endpoint). Phase 08.2 also extends `/ready` + `/server_info` for the DRAINING state.

**Framing deviation (all seven admin endpoints).** envoy-go's `net/http` server emits `Content-Length` (the body is buffered before write); upstream Envoy v1.37.2 emits `transfer-encoding: chunked`. The differential harness dechunks upstream responses before byte-comparing the body. This deviation was first documented for `/ready` at phase 01 (per ADR-0015 paragraph 3) and extends unchanged to all seven endpoints. No allow-list entry; the dechunk is structural.

**Header set (all seven admin endpoints, post-framing-normalization).** The lowercase wire-form header set is `content-type`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `date: <IMF-fixdate>`, `server: envoy` (per ADR-0014). All seven endpoints emit this set. The differential harness uses the existing case-insensitive header comparator (introduced for phase 01).

**Method discrimination posture.** Upstream Envoy v1.37.2 does NOT enforce method discrimination on the six 08.1 read-only endpoints (POST/PUT/DELETE return 200 with the same body as GET — empirical pin in 08.1 SPEC §11.8). envoy-go matches Envoy parity on those six endpoints (no method check; Go stdlib `http.ServeMux` dispatches on path only). The 08.2 mutating endpoint `POST /drain_listeners` DOES enforce method discrimination: non-POST methods return 405 with body `Method <METHOD> not allowed, POST required.\n` per 08.2 SPEC §11.4 empirical pin (ADR-0093 partially amending ADR-0090).

### /ready

*Introduced by phase 01. Justified by ADR-0015 (pre-init contract) and ADR-0014 (Server header value). Captured evidence: `docs/envoy-go/phases/01-static-bootstrap-config/upstream-ready-observation.md`.*

#### Ready-state response (authoritative)

Upstream Envoy v1.37.2 emits (in wire order, lowercase header names):

- **Status line:** `HTTP/1.1 200 OK`
- **Body:** `LIVE\n` (5 bytes: `0x4c 0x49 0x56 0x45 0x0a` — trailing LF, NOT CRLF)
- **content-type:** `text/plain; charset=UTF-8`
- **cache-control:** `no-cache, max-age=0`
- **x-content-type-options:** `nosniff`
- **date:** RFC 7231 IMF-fixdate (non-deterministic; see allow-list above)
- **server:** `envoy` (no version suffix — per ADR-0014)
- **transfer-encoding:** `chunked` (upstream framing)

The phase-01 envoy-go subject emits byte-exact equivalent headers + body, with ONE documented framing deviation: the subject emits `Content-Length: 5` instead of `transfer-encoding: chunked`. The differential harness (Task 14) dechunks upstream responses to the raw body bytes before comparison, neutralising the framing difference. Upgrading the subject to chunked framing is a phase-02+ follow-up, not a phase-01 gate.

Header name case: HTTP/1.1 header names are case-insensitive per RFC 7230 §3.2. Upstream emits lowercase (`content-type`); the envoy-go subject emits Go `net/http` canonical case (`Content-Type`). The differential diff (Task 14) compares header names case-insensitively.

#### Pre-init response

Per ADR-0015, the pre-init `/ready` window is not exercised by the phase-01 differential test — `cmd/envoy-go` fires `admin.MarkReady` before printing the harness readiness sentinel, so the harness only observes the ready state. The subject emits a documented-but-test-irrelevant pre-init response:

- **Status line:** `HTTP/1.1 503 Service Unavailable`
- **Body:** `PRE_INITIALIZING\n` (17 bytes)
- **Content-Length:** `17`
- Other headers as in the ready response (`Content-Type`, `Cache-Control`, `X-Content-Type-Options`, `Server`, `Date`).

Upstream v1.37.2's actual pre-init bytes were unobservable from the minimal bootstrap used in Task 7 (60 probes across two tight loops captured no non-200 response). A later phase that successfully captures upstream pre-init bytes supersedes this subsection via a new ADR.

**DRAINING-state response (08.2 NEW).** When `drain.Manager.State() == DRAINING`, the handler returns 503 Service Unavailable with body `DRAINING\n` (9 bytes; uppercase `DRAINING` followed by single newline) per 08.2 SPEC §11.2 empirical pin. The DRAINING check has precedence over both LIVE and PRE_INITIALIZING — once drain has fired, /ready returns the DRAINING body even if MarkReady has been called and even if /server_info would otherwise return state="LIVE". Header set inherits the umbrella rules.

**Empirical evidence (verbatim Envoy v1.37.2 `/ready` during DRAINING):** see 08.2 SPEC §11.2.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 in DRAINING state. Status 503 byte-equal.

**Forward-pointer note.** ADR-0015 (pre-init contract for /ready) is **partially superseded by ADR-0097**: the LIVE / PRE_INITIALIZING two-state coverage extends to LIVE / PRE_INITIALIZING / DRAINING three-state coverage. ADR-0015's verbatim pre-init body and pre-init status are preserved; ADR-0097 adds the DRAINING branch and the precedence rule.

### /stats/prometheus

See `## Stat-name mapping` for the body-shape contract (Prometheus text exposition format with the SN1–SN8 flattening rules per ADR-0061). Header set + framing inherit the umbrella rules above.

### /config_dump

**Body shape.** `application/json` via `protojson.MarshalOptions{Multiline: true, Indent: " ", UseProtoNames: true, EmitUnpopulated: true}` over `*adminv3.ConfigDump{Configs: []*anypb.Any{...}}` with three sub-envelopes in this order: `BootstrapConfigDump`, `ListenersConfigDump`, `ClustersConfigDump`. No `dynamic_*` arrays (no xDS).

**Empirical evidence (verbatim Envoy v1.37.2 `/config_dump`, first 50 lines):** see 08.1 SPEC §11.1.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 modulo: `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated` allow-listed (envoy-go emits empty / partial values; Envoy auto-populates).

### /clusters

**Body shape.** `text/plain; charset=UTF-8`. 10 cluster-level lines + 18 per-endpoint lines per cluster. Cluster ordering: alphabetical by cluster name. Endpoint ordering: bootstrap-declared order. envoy-go emits the same Envoy default constants (`1024`, `3`, `healthy`, empty locality, `false`, `0`, `-1`) for fields it does not model (circuit breakers, active health checking, locality tags, success rate); see 08.1 SPEC §5.3 for the verbatim line set.

**Empirical evidence (verbatim Envoy v1.37.2 `/clusters`):** see 08.1 SPEC §11.2.

**Equivalence claim.** Tuple-set equality on `(cluster_name, key, value)` triples. Hot-path counters `cx_total`, `cx_connect_fail`, `rq_total`, `rq_active`, `rq_error` allow ±1 tolerance (round-robin LB distribution skew across the 5-request §7.3 load).

**M-1 carry-forward note.** Cluster-name validation is a pre-existing M-1 vulnerability identified in 07.2 REVIEW; the `<cluster>::<key>::<value>` separator is not escaped. An embedded `::` in a cluster name would corrupt the format. envoy-go matches Envoy parity (Envoy also does not escape); the M-1 fix-when-it-lands closes both surfaces simultaneously.

### /listeners

**Body shape.** `text/plain; charset=UTF-8`. One line per listener: `<listener_name>::<bind_addr>` where `<bind_addr>` is `host:port`. Listener ordering: alphabetical by listener name. Trailing newline.

**Empirical evidence (verbatim Envoy v1.37.2 `/listeners`):** see 08.1 SPEC §11.3.

**Equivalence claim.** Body byte-equal (after framing dechunk). Single line per listener; no additional fields. The JSON form (`?format=json`) is structurally richer (returns `{"listener_statuses": [...]}`); deferred per ADR-0089.

### /server_info

**Body shape.** `application/json` via the same protojson MarshalOptions as `/config_dump`. Field set populates `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `node` (from bootstrap), partial `command_line_options{config_path}`, `hot_restart_version: "disabled"`. State enum (08.2 EXTENDED): `LIVE` (post-MarkReady, drain has not fired), `PRE_INITIALIZING` (pre-MarkReady, drain has not fired), `DRAINING` (drain has fired — supersedes LIVE and PRE_INITIALIZING). `INITIALIZING` is documented in `adminv3.ServerInfo_State` but unreachable in envoy-go's static-bootstrap-only model (08.1 SPEC §11.7).

**Empirical evidence (verbatim Envoy v1.37.2 `/server_info`, first 70 lines):** see 08.1 SPEC §11.4.

**Equivalence claim.** Body byte-equal modulo: `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset on envoy-go side; Envoy emits ~40 fields), `hot_restart_version`, `node.*` (same allow-list as `/config_dump`). The `state` field is byte-equal (`"LIVE"` on both sides post-MarkReady without drain).

**Equivalence claim extension (08.2).** The `state` field IS asserted byte-equal across both proxies in DRAINING (`"DRAINING"` literal, per 08.2 SPEC §11.2 empirical pin). The 08.1 byte-equal claim for `"LIVE"` post-MarkReady is unchanged.

**Forward-pointer note.** ADR-0088 is **amended** by ADR-0098 (NOT superseded — purely additive per ADR-0088 consequence (c) verbatim). The ADR-0088 amendment record adds DRAINING to the enum-coverage table and refers to ADR-0098 for the timing semantics.

### /drain_listeners

*Introduced by phase 08.2. Justified by ADR-0093 (method discrimination; partially amends ADR-0090) and ADR-0097 (drain trigger semantics).*

**Body shape (POST).** `text/plain; charset=UTF-8`. Body verbatim `OK\n` (3 bytes; capital `OK` followed by single newline) per 08.2 SPEC §11.1 empirical pin against Envoy v1.37.2. Status 200 OK. The handler is fire-and-forget — 200 OK is emitted BEFORE drain completes; the operator polls /ready or /server_info to observe drain progress. Idempotent — subsequent POSTs during DRAINING return 200 with the same body without re-firing the drain trigger (sync.Once-guarded internally).

**Method discrimination.** Non-POST methods (GET, PUT, DELETE, HEAD) return `405 Method Not Allowed` with body `Method <METHOD> not allowed, POST required.\n` per 08.2 SPEC §11.4 empirical pin. This is the FIRST admin endpoint in envoy-go with method enforcement; partially amends ADR-0090's no-method-discrimination posture (which applies uniformly to read-only endpoints; ADR-0093 records the qualification).

**`?graceful=true` query-param.** Silently accepted (per ADR-0041's silent-ignore precedent). envoy-go's drain is always graceful by construction (the three-state machine has no non-graceful immediate-stop variant); the query-param has no semantic effect.

**Side effects.** First POST: `drain.Manager.Drain()` called (Live → Draining transition); subsequent POSTs: no-op. The endpoint does NOT trigger process exit — the operator-driven drain stays in DRAINING indefinitely until SIGTERM/SIGINT (or kill -9).

**Cross-trigger note.** Upstream Envoy v1.37.2 separates the listener-side drain (POST /drain_listeners — does NOT flip /ready or /server_info to DRAINING) from the load-balancer-disposition flip (POST /healthcheck/fail — DOES flip /ready and /server_info to DRAINING). envoy-go's MVP UNIFIES these triggers under a single drain manager: POST /drain_listeners DOES flip /ready and /server_info to DRAINING in envoy-go (the BODY shapes match Envoy verbatim per §11.2; the TRIGGERS differ at the wiring level). The differential gate's per-proxy trigger script normalizes per 08.2 SPEC §7.2.

**Empirical evidence (verbatim Envoy v1.37.2 `POST /drain_listeners`):** see 08.2 SPEC §11.1.

**Empirical evidence (verbatim Envoy v1.37.2 `GET/PUT/DELETE/HEAD /drain_listeners`):** see 08.2 SPEC §11.4.

**Equivalence claim.** Body byte-equal to reference Envoy v1.37.2 (after framing dechunk). Method-discrimination behavior asserted byte-equal — non-POST returns 405 with the templated body across both proxies. Header set inherits the umbrella rules.

**Forward-pointer note.** ADR-0090 (no-ACL admin-endpoint security posture; no method discrimination on read-only endpoints) is **partially amended** by ADR-0093: the no-ACL posture is preserved verbatim; the no-method-discrimination posture is qualified to read-only endpoints only.

### Applies to

- phase 08.1 and phase 08.2 envoy-go admin subsystem.
- all seven endpoints: `/ready`, `/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`, `POST /drain_listeners` (08.2 NEW).
- `/ready` DRAINING-state body `DRAINING\n` (503; ADR-0097 partially supersedes ADR-0015).
- `/server_info` DRAINING-state `state: "DRAINING"` (ADR-0098 amends ADR-0088).
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to

- HTTP/2 over admin (admin stays HTTP/1.1).
- TLS on admin (admin stays plaintext).
- Other mutating endpoints — `POST /reset_counters`, `POST /quitquitquit`, `POST /healthcheck/*`, `POST /reopen_logs`, `POST /runtime_modify`, `POST /logging` deferred per ADR-0089.
- JSON form of `/clusters` and `/listeners` — `?format=json` deferred per ADR-0089.
- Query-param filtering on `/config_dump` — `?resource=`, `?mask=`, `?include_eds=` deferred per ADR-0089.
- `RoutesConfigDump`, `SecretsConfigDump`, `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes deferred per ADR-0089.
- Other deferred admin endpoints — `/runtime`, `/certs`, `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention`, `/logging`, `/listeners/<name>/*`, `/init_dump` deferred per ADR-0089.
- ACL / authentication on admin port (no-ACL posture per ADR-0090).
- Method discrimination on read-only endpoints (Envoy parity per SPEC §11.8; 405 enforcement deferred for read-only; mutating endpoint /drain_listeners DOES enforce per ADR-0093).
- Path normalization beyond Go stdlib `http.ServeMux` (trailing-slash returns Go stdlib `404 page not found`, NOT Envoy's admin help page; allow-listed for trailing-slash behavior — envoy-go's body diverges from Envoy's body, but the status code matches).

---

## Graceful drain

The envoy-go drain machinery transitions the process from LIVE → DRAINING → exit (via SIGTERM/SIGINT) or LIVE → DRAINING (via POST /drain_listeners; no exit). The state machine lives in the `internal/drain` package (08.2 NEW; ADR-0091); the drain manager is a single-instance lock-free state machine with three states (LIVE / DRAINING / DRAINED) and an in-flight counter.

### Drain triggers

Two operator workflows trigger drain in envoy-go:

1. **SIGTERM or SIGINT:** drain-then-exit. The signal causes `cmd/envoy-go/main.go`'s top-level context to cancel; the main goroutine then calls `drain.Manager.Drain()`, waits on `drain.Manager.Done()` (or a 30s timeout per ADR-0095), then proceeds to per-cluster connection-pool teardown + listener-socket close + admin server close + access-log flush. The total drain window is bounded by the 30s timeout.

   **Deliberate divergence from Envoy v1.37.2** (per 08.2 SPEC §11.7 empirical pin): upstream Envoy v1.37.2 SIGTERM is immediate-exit-without-drain (the log shows `caught ENVOY_SIGTERM` → `shutting down server instance` → `exiting` within ~7ms; no drain delay). envoy-go's design choice (ADR-0092) is to honor the operator-ergonomic expectation that SIGTERM = graceful drain (the dominant Kubernetes / cluster-orchestrator workflow). The differential equivalence claim does NOT exercise the SIGTERM path; the divergence is contract-level.

2. **POST /drain_listeners admin endpoint:** drain-without-exit. The handler triggers `drain.Manager.Drain()` synchronously and returns 200 OK before drain completes. The proxy stays running in DRAINING indefinitely; the operator separately issues SIGTERM/SIGINT (or kill -9) at a later time to actually exit.

Both triggers result in the same drain BEHAVIOR (state transition, listener stop-accepting, in-flight completion, /ready and /server_info responses). They differ only in the post-drain disposition (exit vs. stay-running).

### Drain semantics

When drain fires (state transitions LIVE → DRAINING):

- **New connections rejected via accept-then-FIN.** The Listener Accept loop's fast-path checks `drain.Manager.IsDraining()` on each iteration; an Accept-ed conn during DRAINING is immediately closed (`conn.Close()` → kernel sends FIN) without filter-chain dispatch. Per 08.2 SPEC §11.5 empirical pin: the TCP 3-way handshake completes; the client observes "Empty reply from server" on its first read attempt. NOT listener-socket-close (which would produce kernel RST-on-no-listener for new connections).

- **In-flight requests complete normally.** The HCM filter chain's `decodeHeaders`/`encodeFinalize` pair calls `drain.Manager.Inc()`/`Dec()` to track per-request in-flight count; the drain manager's `Done()` channel closes when the in-flight counter reaches 0 (or the 30s timeout fires). Per 08.2 SPEC §11.3 empirical pin: in-flight HTTP/1.1 requests during drain receive full body delivery with status 200 (no abort), and the response carries NO `Connection: close` header — the keep-alive connection remains open after the response (subsequent requests on the same conn extend the drain window via further Inc calls; deliberate MVP simplification, per-conn drainable-close-at-next-idle-window deferred).

- **TCP-proxy connections complete at connection-close.** TCP-proxy filter's `OnNewConnection`/`OnConnectionClose` pair calls `Inc()`/`Dec()` per connection (correct because TCP-proxy has no per-request semantic).

- **/ready returns 503 DRAINING\n** (per 08.2 SPEC §11.2 verbatim). Operators / load balancers observe the DRAINING signal and stop sending traffic.

- **/server_info returns `state: "DRAINING"`** (per ADR-0098 amending ADR-0088).

- **Idempotent.** Subsequent Drain() calls (e.g., a second POST /drain_listeners, or SIGTERM after a prior /drain_listeners) no-op — the state transition has already fired (sync.Once-guarded).

### Drain timeout

The drain timeout is a hardcoded 30s in envoy-go MVP (per ADR-0095). Envoy v1.37.2's default is 600s (per 08.2 SPEC §11.7 + 08.1 SPEC §11.4 verbatim `"drain_time": "600s"`). The divergence is deliberate to keep test-suite cost tractable; the drain BEHAVIOR is the equivalence claim, not the timeout VALUE. Operator-knob to configure the timeout is deferred to a future runtime / hot-restart family phase.

The drain strategy in upstream Envoy v1.37.2 is `"Gradual"` (the only strategy in the v1.37.2 default-config flow per 08.2 SPEC §11.7 + 08.1 SPEC §11.4). envoy-go's drain is graceful-by-construction (no IMMEDIATE strategy); the strategy concept is not modeled.

### Connection-level drain semantics

Phase 08.2 does NOT implement per-connection drainable closure at next-idle-window (Envoy supports this via `drain_strategy: "Gradual"`'s back-off). HTTP/1.1 keep-alive connections during drain do NOT receive `Connection: close` on the in-flight response (per 08.2 SPEC §11.3 empirical pin matching Envoy parity); subsequent requests on the same conn during DRAINING are processed normally (extending the drain window). HTTP/2 connections during drain emit GOAWAY at drain-trigger (envoy-go MVP design choice; not asserted differentially per 08.2 SPEC §2.1 deferral note).

### Drain manager API surface

- `internal/drain.New(timeout time.Duration) *drain.Manager` — constructor; state initialized to Live.
- `(m *Manager).State() drain.State` — atomic load; returns Live or Draining.
- `(m *Manager).Drain()` — sync.Once-guarded; transitions Live → Draining; arms the Done rendezvous.
- `(m *Manager).Done() <-chan struct{}` — channel closes when inflight reaches 0 after Drain has fired.
- `(m *Manager).Inc()` / `(m *Manager).Dec()` — atomic increment/decrement of inflight counter.
- `(m *Manager).IsDraining() bool` — Listener Accept-loop fast-path; equivalent to State() == Draining.
- `(m *Manager).Timeout() time.Duration` — returns the configured timeout (read-only).

### Applies to

- phase 08.2 envoy-go drain subsystem.
- the SIGTERM/SIGINT-handler in `cmd/envoy-go/main.go` (ADR-0092; deliberate divergence from Envoy parity).
- the POST /drain_listeners admin endpoint (ADR-0093; method discrimination per Envoy parity).
- the /ready DRAINING-state body (ADR-0097; partially supersedes ADR-0015).
- the /server_info DRAINING-state field (ADR-0098; amends ADR-0088).
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to

- Hot restart / parent-child handoff (deferred to runtime / hot-restart family per ADR-0099).
- POST /quitquitquit endpoint (semantic overlap with SIGTERM + /drain_listeners; deferred per ADR-0089 + ADR-0099).
- POST /healthcheck/fail endpoint (envoy-go MVP unifies the listener-drain and load-balancer-disposition triggers under /drain_listeners + the drain manager; /healthcheck/fail stays deferred per ADR-0089).
- Per-listener selective drain (`/listeners/<name>/drain` admin sub-routes deferred per ADR-0089).
- `drain_strategy` per-listener (default GRADUAL only; IMMEDIATE strategy deferred).
- Configurable drain timeout (hardcoded 30s; operator-knob deferred per ADR-0095).
- Per-connection drainable closure at next-idle-window.
- Drain manager interaction with xDS (no xDS yet; deferred).
- HTTP/3 drain semantics (no H3 in MVP; deferred to HTTP/3 + QUIC family).
- Drain progress JSON body on /server_info (envoy-go matches Envoy's empty-of-this-field behavior).
- `Connection: drain` custom response header (Envoy emits no such header per 08.2 SPEC §11.3).
- Multi-instance drain coordination (operator's load-balancer responsibility).

---

## Test harness host networking

Rules governing how fixtures reach a host-side backend from inside the reference Envoy container. These rules are harness-runtime constraints (not equivalence-matrix dimensions), but they are part of the contract because they determine whether a fixture is valid on the pinned reference image.

### DNS — `host.docker.internal` requires `dns_lookup_family: V4_ONLY`

Every fixture whose reference bootstrap uses `host.docker.internal` as a cluster endpoint address MUST set `dns_lookup_family: V4_ONLY` on that cluster.

- **Introduced by:** phase 00 (fixture `0000-tcp-echo`).
- **Justification ADR:** `ADR-0010` in `DECISIONS.md`.
- **Rule:** a `STRICT_DNS` (or other DNS-resolving) cluster whose `socket_address.address` is the literal string `host.docker.internal` must carry `dns_lookup_family: V4_ONLY` at the cluster level.
- **Reason:** Docker Desktop (Linux) resolves `host.docker.internal` to both IPv4 and IPv6 records. Envoy's DNS resolver, left to default (`AUTO`), may select the IPv6 record, which Docker Desktop does not bridge from container to host — the connection fails with "Network is unreachable" and the differential run stalls until the context deadline. `V4_ONLY` eliminates the selection and keeps the fixture deterministic across Docker Desktop, CI runners, and docker-ce hosts.
- **Applies to:** TCP, HTTP/1.1, and HTTP/2 fixtures on this pin.
- **Does not yet apply to:** HTTP/3 / QUIC fixtures — QUIC is UDP and the dual-stack routing behavior differs; the rule must be re-evaluated (via a superseding ADR) when the first QUIC fixture lands.
- **Host-side backend bind address:** because the Docker-provided host gateway on Docker Desktop is a non-loopback IP, host-side test backends MUST bind to `0.0.0.0` (not `127.0.0.1`). A loopback-only bind is unreachable from the container.
- **`extra_hosts` wiring:** CI and developer environments typically need `--add-host=host.docker.internal:host-gateway` on the reference container (set via `testcontainers-go`'s `HostConfigModifier` in `test/differential/harness.go`). Fixtures inherit this from the harness; they do not re-declare it.

Future fixtures that need a different reachability pattern (e.g. container-to-container, or IPv6-on-purpose) add a new rule under this heading via ADR. The V4_ONLY rule above is never silently relaxed — any deviation is an ADR that explicitly supersedes ADR-0010 on the relevant scope.

## TCP proxy

*Introduced by phase 02. Justified by ADR-0024 (per-cluster RR scope) and SPEC §5.4 / §5.5 / §5.8.*

### Response-body byte-equivalence (asserted)

For any fixture whose subject and reference both terminate a TCP connection through `envoy.filters.network.tcp_proxy` (proto `envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy`) backed by a STATIC (subject) or STRICT_DNS (reference) cluster of echo backends, the differential harness compares the concatenated **response bodies** byte-for-byte. Trivially holds for echo backends (each backend reflects the request bytes); for non-echo backends a phase-specific subsection extends this rule.

### Half-close propagation (asserted)

Both proxies must propagate `CloseWrite()` (FIN on the write side) from downstream-to-upstream and from upstream-to-downstream independently — i.e., the dataplane is a true bidirectional pipe with independent half-closes, not a request-response pair. Phase 02 inherits this property from phase 00's `netConn` + `halfClose` byte pump (lifted verbatim per ADR-0023).

### Load-balancer endpoint-selection sequence (NOT asserted)

Cross-proxy LB endpoint-selection sequence is **not** a differential equivalence dimension. Each proxy must be RR-correct **in its own right** (per-proxy distribution): for an N-pick run against a cluster of M endpoints, the per-backend accept count distribution must equal a perfect mod-M partition when `M | N`. This is a **local correctness property** asserted via the optional `DistributionAsserter` interface in `test/differential/fixture/fixture.go`.

The cross-proxy sequence is not asserted because:
- Upstream Envoy's RR LB is per-worker-thread with a randomized starting offset; the absolute sequence of endpoints selected for N consecutive connections is not reproducible across runs or workers.
- The envoy-go subject's RR is per-cluster with a deterministic starting point at index 0 (ADR-0024 + SPEC §5.4 sequence-starts-at-0 invariant); the sequence is reproducible within a single subject process but does not match upstream's randomized sequence.

A phase that needs cross-proxy LB sequence equivalence (e.g., a hash-based LB phase under the load-balancing family) supersedes this subsection with a new ADR documenting the assertion mechanism. Reference-side distribution exactness (fixture `0001-tcp-proxy-rr` and, inherited, `0002-tls-tcp`) depends on the reference container's `--concurrency 1` pin per ADR-0028.

### Listener-bind error semantics (asserted)

If any listener fails to bind, neither proxy should partially serve. Upstream Envoy aborts startup with a non-zero exit and a diagnostic on stderr; phase-02 envoy-go's `cmd/envoy-go/main.go` calls `log.Fatalf("listener start: %v", err)` with the same effect. The two are not byte-compared (`log.Fatalf` and Envoy's startup-error format differ visibly), but both proxies' `/ready` admin endpoint never reports ready in this case (the subject's `MarkReady()` is never reached).

### Applies to

- Phase-02 envoy-go `internal/listener` + `internal/cluster` + `internal/filter/tcpproxy` packages, exercised via fixtures `0000-tcp-echo` and `0001-tcp-proxy-rr`.

### Does not yet apply to

- Multiple network filters in a single filter_chain (e.g., chained `redis_proxy + tcp_proxy`) — Network filters family. Multiple listener filters in a `listener_filters[]` pipeline IS supported as of 07.2 — see `## Listener filters`.
- TLS — phase 03.
- HTTP-aware proxying (`HttpConnectionManager`) — phase 04+.
- LB policies other than ROUND_ROBIN — load-balancing family.
- Cluster types other than STATIC (subject side) / STRICT_DNS (reference side, per ADR-0010) — later phases.
- Health-check-driven endpoint selection — upstream-robustness family.

---

## TLS

*Introduced by phase 03. Justified by ADR-0031 (stdlib crypto/tls stack selection), ADR-0030 (TLS parameter mapping scope), ADR-0033 (filter-chain subset supersedes ADR-0025), ADR-0032 (upstream TLS dialer), ADR-0035 (fixture-0002 differential scope), ADR-0036 (this subsection).*

Phase 03 introduces envoy-go's first cryptographic surface: downstream TLS termination, upstream TLS origination, and SNI-based filter-chain dispatch. This subsection codifies what the differential harness compares across the reference and subject proxies over TLS, and — importantly — what it does NOT compare.

Fixture `0002-tls-tcp` exercises downstream TLS termination + SNI routing + per-cluster RR distribution. Upstream TLS origination code paths are delivered in the runtime (ADR-0032's `Cluster.Dial` TLS branch; ADR-0031's stdlib `crypto/tls` stack) and unit-tested via `internal/cluster/cluster_test.go` + `internal/tls/config_test.go` + the `FuzzTLSContextParse` upstream seed, but are NOT exercised by a phase-03 differential fixture. See ADR-0035 for the scope-reduction rationale (the phase-03 harness has plain-TCP backends; upstream-TLS differential coverage awaits a later phase with TLS-capable backends).

### Asserted equivalence

**Plaintext-after-decryption byte equivalence.** For a TLS-terminated downstream connection, the response body observed by the fixture driver (after the tunnel is fully peeled) must be byte-exact between reference and subject. Fixture `0002-tls-tcp` exercises this surface with 9 TLS round-trips per SNI per side, 18 per proxy, over a plain-TCP echo upstream.

**Per-SNI chain-selection equivalence.** Given the same SNI on the ClientHello, both proxies must select the logically-equivalent filter chain and dispatch to the logically-equivalent upstream cluster. This is witnessed indirectly via the distribution assertion: fixture 0002's `[3,3,3]` per cluster per SNI per side implies the SNI → chain → cluster dispatch is consistent.

**Server-certificate identity by SNI.** For a given ClientHello SNI, the server certificate selected by each proxy must match on SAN identity. Phase 03 does not byte-compare the cert bytes (both proxies serve the same committed PEM in fixture 0002 — the byte-compare is trivially equal); the rule is semantic: both pick the cert whose SAN covers the SNI.

### Not asserted

**Upstream SNI + CA equivalence (unit-tested only, not differentially asserted).** Because fixture 0002 does not exercise upstream TLS (ADR-0035), the "both proxies send the same SNI / validate against the same trusted_ca" property is covered only by `internal/cluster/cluster_test.go` unit tests and `internal/tls/config_test.go`. A future phase with TLS-capable backends (or a later HTTPS fixture) will either extend this subsection with the assertion or supersede via a new ADR.

**Encrypted-side byte equivalence.** Neither the TLS record boundaries, the session-ticket material, session-ticket-key rotation timing, TLS 1.3 cipher selection (Go's `crypto/tls` and Envoy's BoringSSL have different defaults), handshake message byte ordering/timing, server random, session IDs, nor any other encrypted-side observable is compared. The differential harness diffs decrypted bytes, not TLS records.

**Negotiated ALPN value.** `alpn_protocols` on both sides is passed through to `stdtls.Config.NextProtos`, so both proxies advertise the same ALPN offers; the negotiated value (which wins the ALPN negotiation) is not surfaced to the fixture driver in phase 03. If a later phase asserts ALPN negotiation, it adds a fixture opt-in and extends this subsection.

**Handshake-layer timing.** Not asserted. TLS handshake completion time varies with cipher selection, session resumption state, and handshake retries.

### Parameter mapping caveats

Two `tls_params` fields do not round-trip with full fidelity between Envoy's BoringSSL and Go's `crypto/tls` (see ADR-0030):

- `cipher_suites` with TLS-1.3 cipher names (e.g., `TLS_AES_128_GCM_SHA256`): Go's `crypto/tls` does not permit TLS-1.3 cipher selection. envoy-go logs a diagnostic and drops the entry. Negotiated TLS-1.3 cipher may differ between proxies; this is within the "encrypted-side not asserted" rule above.
- `signature_algorithms`: not publicly configurable in `crypto/tls`. envoy-go errors at parse if a fixture sets this field. Phase-03 fixtures do not set it.

### Scope boundaries

Phase 03 does NOT implement session resumption assertion, OCSP stapling, mTLS validation on the downstream side, SDS, SPIFFE / custom validators, post-quantum key exchange, HTTPS (HTTP over TLS — phase 04+), upstream TLS differential assertion (deferred per ADR-0035), or transport socket types beyond `tls`. See SPEC §2 for the full non-purposes list and the phase each is deferred to.

ALPN-driven codec selection inside `Filter.Handle` (per ADR-0050; HCM-internal AUTO-codec dispatch) is the TLS-layer ALPN consumer; it remains in scope for the TLS section. As of phase 07.2 the listener-side ALPN consumer (chain-match against `application_protocols` populated by `tls_inspector`) is also in scope, but lives in `## Listener filters`. The two ALPN consumers coexist orthogonally per ADR-0083: `tls_inspector` populates `inputs.ApplicationProtocols` from the ClientHello before chain-match selection (chain selects which `filter_chain` runs); ADR-0050's HCM-internal codec dispatch then chooses HTTP/1.1 vs HTTP/2 inside whichever HCM filter the selected chain wires up. ADR-0050 is NOT superseded by ADR-0083 / ADR-0081.

See `## Listener filters` for the listener-side filter primitives (the listener-filter dispatch pipeline, the 8-dimension `FilterChainMatch` algorithm, and `Listener.default_filter_chain` fallback semantics — all in scope as of phase 07.2).

---

## HTTP/1.1

Phase 04 introduces HTTP/1.1 routing — the HCM network filter parses request lines off the downstream connection, matches against an inline route table (`match.prefix` bytewise OR `match.path` case-sensitive exact), dispatches through a `direct_response` (HCM-locally-generated reply) or a `route` (router action over a per-request fresh upstream dial). See ADR-0044.

### Asserted equivalence

- Response status code per request (across the full 27-request workload of fixture 0003).
- Decoded response body bytes for `direct_response` 2xx paths (the 9 × `/health` → 200 `OK\n` requests).
- Route-match selection: same method + path → same matched route on both proxies, witnessed by per-cluster RR distribution `[3,3,3]` over the router-action subset (the 9 × `/api/v1/<n>` requests on the subject side).
- Upstream-side request preservation: verbatim Host, method, path-with-query, body — except where stdlib HTTP/1.1 parsing on the subject side introduces a bounded, documented normalisation per ADR-0037.

### Not asserted

- Decoded response body bytes for routed-to-upstream requests. Rationale: the reference proxy (STRICT_DNS) and the subject proxy (STATIC) may start their RR distribution at different endpoint indices. Both produce `[3,3,3]` overall, but request[i] may hit a different backend on each side, yielding non-equal `backend-<idx>:v1/<n>` body bytes per request even though the routing behaviour is correct on both sides. Status code + per-side `[3,3,3]` distribution are the witnesses; per-request body equivalence would require synchronising RR start indices, which is out of scope.
- Local-reply body bytes for 4xx/5xx (Envoy emits HTML/JSON local replies; envoy-go emits plain-text bodies like `"not found\n"`. Status is asserted; body is relaxed).
- Response-header **value** equality (set-equality modulo allow-list only).
- `Content-Length` vs `Transfer-Encoding: chunked` framing per response (the harness decodes both via `http.ReadResponse`).
- Upstream connection re-use (envoy-go does not pool per ADR-0039; Envoy does).
- `x-envoy-*` / `x-forwarded-*` / `x-request-id` headers (envoy-go injects none; Envoy injects many — all in the allow-list).

### Header allow-list extensions

See the `## Header allow-list` table above, rows added by ADR-0044: `Server`, `Content-Length`, `Transfer-Encoding`, `x-envoy-*`, `x-forwarded-*`, `x-request-id`.

### Applies to

- Phase-04 envoy-go `internal/filter/hcm/` package, exercised via fixture `0003-http11-routing`.
- The phase-04 HCM-filter chain shape requirement that the chain be non-empty with router as terminal entry; the original ADR-0042 "exactly `[router]`" rule was partially superseded by ADR-0071 in phase 07.1 (lower bound stays; upper bound lifted) — see `## HTTP filter chain` for the full discipline.
- `match.prefix` (bytewise) and `match.path` (case-sensitive exact) only.

### Does not yet apply to

- HTTP/2 (phase 05).
- HTTP/3 (later).
- The full HTTP-filter chain framework (iteration protocol, async-resume, sendLocalReply semantics, per-route config) — see `## HTTP filter chain`.
- Upstream connection pooling (upstream-robustness family).
- HTTPS (phase 04.x or 05.x or a dedicated HTTPS-fixture sub-phase).
- `match.regex` / `match.path_separated_prefix` / `match.connect_matcher` / header-aware match / query-parameter-aware match (subset enforcement — ADR-0038).
- HTTP-filter iteration protocol (decode-headers, decode-data, encode-headers, etc. — phase 07).

---

## HTTP/2

*Introduced by phase 05.1. Justified by ADR-0046 (codec source: x/net/http2.Framer + hpack), ADR-0047 (server settings defaults), ADR-0048 (server connection manager from scratch), ADR-0050 (ALPN dispatch wiring), ADR-0051 (h2spec threshold + pin), ADR-0052 (this subsection — SCAFFOLD form for 05.1, in-place edited for 05.2).*

*Extended by phase 05.2. Justified by ADR-0055 (flow-control discipline), ADR-0056 (per-request fresh upstream H2 dial), ADR-0057 (closes ADR-0035 H2 leg via fixture 0004), ADR-0058 (trailers observed but not forwarded; carry-forwards M-4 + M-10).*

Phase 05.1 introduced envoy-go's downstream HTTP/2 dataplane; phase 05.2 closes the dataplane on the upstream side: cluster-side HttpProtocolOptions parsing, Cluster.DialH2 + ClientConn + RoundTrip, routerActionH2 action variant, and the project's first full-stack HTTPS h2 differential fixture (0004-h2-routing) closing ADR-0035 H2 leg. The flow-control discipline tightening per ADR-0055 makes the codec primitives load-bearing for realistic H2 workloads.

*Phase-07.1 HCM filter-chain framework note: phase 07.1 wires the H2 dispatch path through the same `internal/filter/http` iteration protocol as H1 (per ADR-0071); the original ADR-0040 (HCM-direct-call subset) is totally superseded and ADR-0042's "exactly `[router]`" upper bound is partially superseded — see `## HTTP filter chain` for HCM filter-chain dispatch wiring.*

### Asserted equivalence (05.1 + 05.2 scope)

- `:status` per request: required + asserted on every fixture-0004 request (h2spec section 8 covers indirect coverage on the codec; fixture-0004 covers the full proxy+upstream surface).
- Decoded body bytes on `direct_response` 2xx paths: byte-equal to the configured body string. Witnessed by fixture 0004's 9 `/health` requests on both sides + envoy-go's hcm-package unit tests.
- Per-stream response header set-equality modulo allow-list: `:status` (required + asserted), `Server` (matched verbatim with upstream's `envoy` per ADR-0014), `Content-Type`, `Content-Length`, `Date` (presence required; value not byte-compared). NEW (05.2): routed-to-upstream H2 responses now in scope — `:status` required + asserted; `Server`/`Content-Type`/`Content-Length`/`Date` headers from the upstream backend forwarded verbatim (no router-injected headers); the per-stream response header set-equality between sides asserted modulo the same allow-list.
- NEW (05.2): routed-to-upstream H2 request preservation — `:method`/`:path`/`:scheme`/`:authority` forwarded verbatim from downstream to upstream (witnessed by the in-process backend in fixture 0004's tests asserting on received pseudo-headers). The path normalisation discussed in master phase-05 SPEC §5.7 is empty on the H2 side (the path is the bytes of the `:path` pseudo-header — there's no stdlib net/http parsing to inject normalisations).
- NEW (05.2): route-match selection equivalence on H2 — same method + path → same matched route on both proxies (witnessed indirectly by per-side `[3,3,3]` distribution + `:status` per request).
- NEW (05.2): per-cluster RR distribution `[3, 3, 3]` per side over the 9 router-action requests (local-correctness; cross-side sequence is NOT asserted, mirroring phase-04's relaxation per ADR-K extended to H2).
- NEW (05.2): ALPN selection equivalence at the differential level — a downstream client advertising only `h2` reaches the H2 driver on both proxies (witnessed by fixture 0004's `:status 200` on every routed response).

### Not asserted (05.1 + 05.2 scope)

- Wire-byte H2 framing — unchanged from 05.1.
- SETTINGS values byte-for-byte — unchanged from 05.1.
- WINDOW_UPDATE timing or count — unchanged from 05.1; ADR-0055's tightening adds frame counts that depend on body size and peer window behaviour, which are inherently non-deterministic across the two proxies.
- Stream id allocation pattern — unchanged from 05.1.
- Trailers — observed but not forwarded per ADR-0058 (formalises the upstream-side discard rule; the 05.1 server-side rule was already trailers-not-forwarded).
- 0-RTT TLS early-data behaviour — unchanged from 05.1.
- NEW (05.2): connection re-use upstream (per ADR-0056) — Envoy pools, envoy-go does not; both produce the same per-request `:status`/body output; cross-conn frame counts differ.
- NEW (05.2): cross-side request body bytes for routed-to-upstream requests (mirror of phase-04's ADR-K relaxation extended to H2) — fixture 0004's 9 `/api` request bodies are bodyless GETs, so this rule is unexercised in 05.2; carried forward as the rule for any future POST/PUT-bearing fixture.

### Header allow-list extensions

See the `## Header allow-list` table above. Rows added by ADR-0052: `:status` (active in 05.1; locally-generated H2 responses), `:method`/`:path`/`:scheme`/`:authority` — phase 05.2 flips the latter four rows from "applies-to: phase 05.2 (forward-looking)" to **"applies-to: phase 05.2 routed-to-upstream H2 (active per ADR-0057)"**.

### h2spec threshold

Sections 3, 4, 5, 6 (excluding 6.6 PUSH_PROMISE), 7, 8 — all `failed == 0`. Pin: `summerwind/h2spec` at the SHA recorded in `CONFORMANCE_PINS.md` per ADR-0051.

**ADR-0055 prose extension (phase 05.2):** the from-scratch H2 codec respects `MaxFrameSize` chunking on outbound DATA, per-stream send-window enforcement, inbound WINDOW_UPDATE emission on a half-window high-water threshold, and overflow bounds-checks on WINDOW_UPDATE deltas. These properties are validated by the regression unit tests (per phase-05.2 PLAN Tasks 2-5) and by the existing h2spec section 5/6 coverage at the pinned SHA; no new section requirements are added.

### Applies to (05.1 + 05.2)

- Phase-05.1: `internal/filter/hcm/h2/` server-side codec (unchanged); the codec-neutral `directResponseAction` factoring; the `--allow-h2c` test-only flag; the conformance suite under `test/conformance/h2spec/`.
- Phase-05.2: `internal/filter/hcm/h2/client.go` (`ClientConn` + `RoundTrip` + `Close`); `internal/cluster/dial_h2.go`; `internal/cluster/manager.go HttpProtocolOptions` reader + validation; `Cluster.UseH2()` accessor; `routerActionH2` action variant in `internal/filter/hcm/actions.go`; fixture `0004-h2-routing` (full-stack HTTPS h2); `test/helpers/h2.go H2RoundTrip` helper.

### Does not yet apply to

- HTTP/3 (later).
- Server push (out of scope permanently in 05.1; potentially out of scope project-wide).
- gRPC framing.
- Trailer forwarding (deferred to phase 07 framework + gRPC family per ADR-0058).
- Upstream H2 stream pooling (upstream-robustness family per ADR-0056).
- h2c production fixtures (test-only path).
- mTLS over h2 (deferred).
- Mixed-codec clusters (a single cluster used by both H1 and H2 listeners — load-balancing family or a future phase explicitly adding mixed-codec clusters).

(REMOVED from this list — now active: routed-to-upstream H2 → active per ADR-0057; fixture 0004 → active per phase-05.2 Task 14.)

---

## HTTP filter chain

*Introduced by phase 07.1. Justified by ADR-0070 (planner-time split), ADR-0071 (iteration protocol shape; supersedes ADR-0040 totally; partially supersedes ADR-0042), ADR-0072 (HTTPRegistry threading), ADR-0073 (typed_per_filter_config 3-tier merge; amends ADR-0041), ADR-0074 (cors + envoy_go_test filter set), ADR-0075 (sendLocalReply encode-chain entry semantics), ADR-0076 (1 MiB buffer cap; 413 on decode overflow; reset on encode overflow; amends ADR-0041).*

### Asserted equivalence
- cors filter preflight response shape (status, header set, header values) byte-equal to reference Envoy v1.37.2 — verbatim scrape pinned in `### Empirical evidence (cors preflight)` below.
- cors filter actual-request response header injection byte-equal.
- Filter declaration order on decode side; reverse on encode side — verbatim scrape evidence pinned in `### Empirical evidence (filter ordering)` below.
- 413 Payload Too Large response shape on decode-side buffer overflow — verbatim scrape evidence pinned in `### Empirical evidence (413 overflow)` below.
- typed_per_filter_config 3-tier merge precedence (Route > VirtualHost > RouteConfiguration); most-specific override (no field-merge).
- sendLocalReply enters encode chain at filter[len-1] of the encode-side filter set (reverse iteration start); full encode chain runs — verbatim scrape evidence pinned in `### Empirical evidence (sendLocalReply entry)` below.

> **Phase 10 forward-pointer (per ADR-0110).** Phase 10 (`envoy.filters.http.header_mutation`) introduces the **multi-tier evaluation** model (per ADR-0110 amending ADR-0073). The default model remains most-specific-override per ADR-0073 (used by cors, fault); filters whose proto semantics demand multi-tier evaluation use the framework's `RequestRouteConfigsAllTiers` callback + `PerRouteConfig.ResolveAllTiers` accessor, opting into the multi-tier model per ADR-0110.

### Not asserted
- Behavioral equivalence of the test-only `envoy.filters.http.envoy_go_test` probe filter — structural assertion only (no reference Envoy implements it).
- Watermark backpressure event timing (out of MVP scope).
- 1xx informational header processing (out of MVP scope).
- Metadata frame processing (out of MVP scope).
- ContinueAndDontEndStream status semantics (out of MVP scope).
- Per-route filter `disabled` flag honoring (out of MVP scope).

### Buffer overflow behavior
- decode-side: 413 local reply, hardcoded constant 1 MiB (filterBufferLimitBytes), enters encode chain.
- encode-side: connection reset (H1: close; H2: RST_STREAM).
- per_connection_buffer_limit_bytes / per_request_buffer_limit_bytes silently ignored — extends ADR-0041 silent-ignore set.

### Async resume mechanics
- StopIteration parks dispatch goroutine on a per-stream resume channel.
- ContinueDecoding / ContinueEncoding callbacks unblock the channel.
- Single-goroutine-per-request iteration; no per-filter goroutine spawned by the framework.
- ctx.Done() during pause aborts iteration; OnDestroy fires for cleanup.

> **Phase 09 forward-pointer (per ADR-0102).** Phase 09 (`envoy.filters.http.fault`) is the FIRST production exerciser of the async-resume primitive on the request side; see `### envoy.filters.http.fault ### Async-resume mechanics` for fault-specific details (the `time.AfterFunc` timer + `cb.SendLocalReply` + `cb.ContinueDecoding` parkDecode-wake-up mechanics; the chain's `localReplyDone` gate short-circuits the resumed iteration without dialing the upstream). The 07.1 `envoy.filters.http.envoy_go_test` probe filter is the structural-coverage exerciser.

### Filter ordering
- http_filters[] declaration order on decode-side.
- Reverse declaration order on encode-side (router last on decode → router first on encode).
- Last entry MUST be the router (terminal filter); errors at parse otherwise.

### Empirical evidence (filter ordering)

**Probe configuration:** chain `[lua_a, lua_b, envoy.filters.http.router]` where `lua_a` and `lua_b` log on decode/encode entry via Envoy's Lua filter (`logCritical` writes to Envoy's stderr at `[critical]` level). Listener `127.0.0.1:10000`; route `/` → STRICT_DNS cluster `c0` reaching a host-side nginx backend.

**Probe request:** `GET / HTTP/1.1` (single sequential request).

**Verbatim Envoy stderr trace (decoded/encoded line emit order; timestamps preserved):**

```
[2026-05-01 01:10:55.841][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=A index=0
[2026-05-01 01:10:55.841][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=B index=1
[2026-05-01 01:10:55.842][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=B index=1
[2026-05-01 01:10:55.842][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=A index=0
```

**Conclusion (pinned):** decode-side iteration is in declaration order (`lua_a` index 0 → `lua_b` index 1 → router index 2 implicitly terminal). Encode-side iteration is in **reverse** declaration order (`lua_b` index 1 → `lua_a` index 0; router has no encode-side log emission in this probe but is the entry point into the encode chain since it produces the upstream response). This is the empirical evidence for the §6.6 + §5.5 reverse-encode-order rule. envoy-go's `chain.runEncodeHeaders` / `runEncodeData` / `runEncodeTrailers` MUST iterate from `len(filters)-1` down to `0`.

### Empirical evidence (cors preflight)

**Probe configuration:** chain `[envoy.filters.http.cors, envoy.filters.http.router]`; one virtual_host with `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{allow_origin_string_match: [exact: "https://example.test"], allow_methods: "GET, POST, OPTIONS", allow_headers: "x-foo, x-bar", expose_headers: "x-baz", allow_credentials: true, max_age: "600"}`; one route `/` → STRICT_DNS cluster `c0` reaching a host-side nginx backend (which serves a 200 + HTML body on `GET /`).

**Probe (a) — Preflight, allowed origin:**

Request: `OPTIONS / HTTP/1.1` `Origin: https://example.test` `Access-Control-Request-Method: GET` `Access-Control-Request-Headers: x-foo,x-bar`

Verbatim response (header set in wire order, lowercase as emitted by Envoy):

```
< HTTP/1.1 200 OK
< access-control-allow-origin: https://example.test
< access-control-allow-credentials: true
< access-control-allow-methods: GET, POST, OPTIONS
< access-control-allow-headers: x-foo, x-bar
< access-control-max-age: 600
< access-control-expose-headers: x-baz
< date: Fri, 01 May 2026 01:09:51 GMT
< server: envoy
< content-length: 0
```

**Probe (b) — Preflight, disallowed origin:**

Request: `OPTIONS / HTTP/1.1` `Origin: https://other.test` `Access-Control-Request-Method: GET`

Verbatim response:

```
< HTTP/1.1 405 Method Not Allowed
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 157
< x-envoy-upstream-service-time: 0
```

**Probe (c) — Actual GET, allowed origin:**

Request: `GET / HTTP/1.1` `Origin: https://example.test`

Verbatim response (CORS-relevant headers shown in wire order; full body omitted — body is the upstream nginx default-page):

```
< HTTP/1.1 200 OK
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 896
< last-modified: Tue, 07 Apr 2026 12:09:53 GMT
< etag: "69d4f411-380"
< accept-ranges: bytes
< x-envoy-upstream-service-time: 0
< access-control-allow-origin: https://example.test
< access-control-allow-credentials: true
< access-control-expose-headers: x-baz
```

**Probe (d) — Actual GET, no Origin:**

Request: `GET / HTTP/1.1` (no Origin header)

Verbatim response (CORS headers absent — passthrough confirmed):

```
< HTTP/1.1 200 OK
< server: envoy
< date: Fri, 01 May 2026 01:09:51 GMT
< content-type: text/html
< content-length: 896
< last-modified: Tue, 07 Apr 2026 12:09:53 GMT
< etag: "69d4f411-380"
< accept-ranges: bytes
< x-envoy-upstream-service-time: 0
```

**Conclusions (pinned):**

- **Preflight, allowed origin (probe a):** status `200 OK` (NOT `204 No Content` — BRAINSTORM §2.4 hypothesized 204; v1.37.2 emits 200). Six CORS response headers in this order: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-allow-methods`, `access-control-allow-headers`, `access-control-max-age`, `access-control-expose-headers`. Body length 0. envoy-go's cors filter MUST emit `200 OK` with empty body and the same six headers in the same order.
- **Preflight, disallowed origin (probe b):** the cors filter does NOT synthesize a 4xx local-reply for disallowed-origin preflights. Instead, the preflight passes through to the router, which sees an `OPTIONS /` and responds `405 Method Not Allowed` (since the route doesn't accept OPTIONS — which is the v1.37.2 default for routes without `route.connect_matcher` or explicit options handling). envoy-go's cors filter MUST replicate this passthrough (do NOT inject a 4xx; let the request flow to the router).
- **Actual request, allowed origin (probe c):** the cors filter's encodeHeaders adds three CORS response headers to the upstream response: `access-control-allow-origin`, `access-control-allow-credentials`, `access-control-expose-headers`. (NOT all six — `allow-methods`/`allow-headers`/`max-age` are preflight-only.)
- **Actual request, no Origin (probe d):** the cors filter is a no-op (no CORS response headers injected). Confirms the filter's gating discipline (no Origin → no encode-side action).

> **Phase 10 forward-pointer.** Phase 10 (`envoy.filters.http.header_mutation`) is the SECOND production filter to mutate response headers in `EncodeHeaders` — see `### envoy.filters.http.header_mutation` for the programmable-mutation discipline. Cors injects a fixed 3-header set; header_mutation runs a programmable AppendAction × 4 + Remove pipeline.

### envoy.filters.http.fault

#### Asserted equivalence (per phase 09 SPEC §11)

When `envoy.filters.http.fault` is present in `http_filters`, envoy-go MUST emit the same response status, body, and 4-header set as reference Envoy v1.37.2 for the canonical fault scenarios.

- **Abort response** (when `abort.percentage` rolls hit and `headers` field matches):
    - Status: `<abort.http_status>` (constrained to `[200, 600)` at config-load time per ADR-0101; out-of-range values cause boot failure).
    - Body: byte-exact `fault filter abort` (18 bytes, NO trailing newline). NO `charset=UTF-8` modifier on the content-type. NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding: chunked` headers.
    - Header set on the wire: `content-length: 18`, `content-type: text/plain`, `date: <IMF-fixdate>`, `server: envoy`.
    - For non-stdlib status codes (e.g. 418), the status text portion is allow-listed per the differential harness (`418 Unknown` upstream vs `418 I'm a teapot` envoy-go stdlib). Status code asserts byte-equal; status text portion is allow-listed.
- **Delay response** (when `delay.percentage` rolls hit and `headers` field matches; no abort):
    - Status: passes through from upstream (typically 200 OK + backend body).
    - Latency: `delay.fixed_delay ± 10ms` (per the timing-tolerance bullet in `## Timing tolerances`).
- **Combined delay + abort** (when both criteria match): delay fires first, then abort fires after the delay completes. The wire response is the abort response (4-header set + `fault filter abort` body), arriving `delay.fixed_delay` after the request.
- **Headers-field gate**: when `headers` is non-empty, fault is only injected if ALL listed name+value pairs match the request. Header NAMES match case-insensitively (per HTTP/1.1); header VALUES match case-sensitively under `string_match.exact`. Other StringMatcher variants (regex, prefix, suffix, contains) are silently ignored at fault-eval time — see Does-not-yet-apply-to.

#### Per-route 3-tier merge (per ADR-0073 + phase 09 SPEC §11.7)

Per-route `typed_per_filter_config` for `envoy.filters.http.fault` WHOLESALE-overrides the listener-level fault config. A per-route HTTPFault that omits `delay` does NOT inherit the listener-level `delay` (and conversely for `abort`, `headers`, `max_active_faults`). Empirically confirmed: a route with `abort: 418, no delay` over a listener with `delay: 300ms` returns instant 418 (1.1ms total), NOT delayed 418 (~301ms).

#### `max_active_faults` concurrency cap

When `max_active_faults > 0`, a per-filter-instance `atomic.Int64` counter caps the in-flight fault count. New faults that arrive at the cap are SKIPPED (the request passes through normally) and the `fault.faults_overflow` counter increments. The cap is per-filter-instance (per `New` factory closure), shared across all requests routed through the same listener filter chain. LBP-1 sixth application — closure-captured `*atomic.Int64` shared counter (per ADR-0105).

#### Async-resume mechanics (per ADR-0102)

The fault filter's delay path uses `time.AfterFunc` to schedule a callback that calls `cb.ContinueDecoding()` after the configured delay. The chain parks at `StopIteration` and resumes from the timer goroutine. On the **combined delay+abort path**, the timer callback calls `cb.SendLocalReply(...)` followed by `cb.ContinueDecoding()` — the `ContinueDecoding` is purely a parkDecode wake-up signal; the chain's `localReplyDone` gate (set by `SendLocalReply`) makes the resumed iteration short-circuit WITHOUT advancing past the parked filter, so the upstream is NEVER dialed and the synthesized abort response is what reaches the wire. `OnDestroy` calls `f.delayTimer.Stop()` to cancel the timer on request teardown (downstream-disconnect, drain-induced stream-reset). The `markedActive` per-instance `atomic.Bool` flag guards the `activeFaults` atomic.Int64 counter Inc/Dec balance against the OnDestroy-races-timer-callback case.

#### Does not yet apply to (per phase 09 deferrals — ADRs 0101, 0103, 0104, 0107)

- **Header-driven fault path** (`x-envoy-fault-{delay,abort}-request[-percentage]`): coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per phase 09 §11.5 empirical pin; both deferred per ADR-0104. envoy-go silently parses the proto sub-messages but does not honor them; the four documented request headers are silently ignored.
- **`response_rate_limit`** (FaultRateLimit token-bucket): deferred to a future fault-extension phase OR the bandwidth_limit filter. The `fault.response_rl_injected` stat is emitted as a permanently-zero counter for differential parity per ADR-0107.
- **`abort.grpc_status`**: deferred to gRPC family.
- **`upstream_cluster`, `downstream_nodes` filters**: deferred to small follow-up phases.
- **All four runtime-key fields** (`*_runtime`): deferred to Runtime + hot restart family.
- **`disable_downstream_cluster_stats`**: deferred to per-downstream-cluster stat fan-out phase (no current ROADMAP row).
- **`filter_enabled` / `filter_enabled_runtime`**: deferred to Runtime + hot restart family.
- **HeaderMatcher non-exact variants** (regex, range, prefix, suffix, contains, present-only): deferred to whatever phase lands the full HeaderMatcher engine.
- **Differential testing under H2 streams**: fixture 0011 is HTTP/1.1-only; H2 differential testing of fault is deferred.

#### Empirical evidence (verbatim curl excerpts from phase 09 SPEC §11.3)

```
$ curl -isS http://127.0.0.1:11000/foo  # delay 100% 100ms + abort 100% 503

HTTP/1.1 503 Service Unavailable
content-length: 18
content-type: text/plain
date: Sun, 03 May 2026 18:33:59 GMT
server: envoy

fault filter abort
```

(Body byte-count = 18; NO trailing newline; 4-header set as above.)

### envoy.filters.http.header_mutation

#### Asserted equivalence (per phase 10 SPEC §11)

When `envoy.filters.http.header_mutation` is present in `http_filters`, envoy-go MUST emit the same post-mutation request headers (visible at the upstream backend) and post-mutation response headers (visible at the downstream client) as reference Envoy v1.37.2 for the canonical mutation scenarios.

- **Request-side mutations** (per `mutations.request_mutations[]`): applied in proto-declared order in `DecodeHeaders` BEFORE the request reaches the upstream router. Each mutation is one of `Remove` (deletes the named header) or `Append` with one of 4 `AppendAction` variants:
    - `APPEND_IF_EXISTS_OR_ADD` (default; enum 0): `headers.Add(name, value)` — multi-valued header gets one more value; absent target gets first value.
    - `ADD_IF_ABSENT` (1): conditional add if and only if the target is absent (`headers.Get(name) == ""`).
    - `OVERWRITE_IF_EXISTS_OR_ADD` (2): `headers.Set(name, value)` — collapses any multi-valued slot to a single value; absent target gets first value.
    - `OVERWRITE_IF_EXISTS` (3): conditional set if and only if the target is present.
- **Response-side mutations** (per `mutations.response_mutations[]`): applied in proto-declared order in `EncodeHeaders` BEFORE the response writes to the wire. Same 4 AppendAction variants + Remove. envoy-go's `EncodeHeaders` runs in REVERSE filter-list order per ADR-0075; header_mutation's response_mutations apply AFTER any later-in-list filter's encode-side mutations.
- **`keep_empty_value` semantics**: when an Append op has `value == ""` and `keep_empty_value=false` (default), the op is a SILENT NO-OP regardless of AppendAction. When `keep_empty_value=true`, the empty value is materialized subject to the AppendAction's conditional gate (e.g., `OVERWRITE_IF_EXISTS` with empty value + keep + absent target = no-op; with present target = replace value with empty).
- **Multi-valued header behavior** (per phase 10 SPEC §11.4): `OVERWRITE_*` collapses multi-valued slots to a single value (the new one). `APPEND_IF_EXISTS_OR_ADD` preserves prior values + adds one more (resulting in N+1 values). Applies to all multi-valued headers including `Set-Cookie`, `Vary`, `Cache-Control`.

#### Multi-tier per-route evaluation (per ADR-0110 + phase 10 SPEC §11.5)

Per-route `typed_per_filter_config` for `envoy.filters.http.header_mutation` is evaluated at ALL THREE tiers (Route, VirtualHost, RouteConfiguration), NOT merged most-specific-only. This is structurally different from cors / fault per-route handling (which use most-specific override per ADR-0073). The cross-tier ordering is controlled by the listener-level `most_specific_header_mutations_wins` flag:

- **`most_specific_header_mutations_wins=false`** (DEFAULT): Listener-level mutations applied FIRST, then per-route tiers in order Route → VirtualHost → RouteConfiguration. RouteConfiguration's mutations are applied LAST → least-specific-wins overlap.
- **`most_specific_header_mutations_wins=true`**: Listener-level mutations applied FIRST, then per-route tiers in REVERSED order RouteConfiguration → VirtualHost → Route. Route's mutations are applied LAST → most-specific-wins overlap.

Each tier's mutations are applied in proto-declared order WITHIN the tier (the cross-tier flag controls only the inter-tier sequence). Listener-level mutations are ALWAYS applied first regardless of the flag.

Empirically confirmed: with listener `x-test=listener`, RouteConfiguration `x-test=rc`, VirtualHost `x-test=vh`, Route `x-test=route` (all OVERWRITE_IF_EXISTS_OR_ADD): flag=false → final `x-test: rc`; flag=true → final `x-test: route`.

#### Protected-header set (per ADR-0111 + phase 10 SPEC §11.1)

Envoy v1.37.2 enforces a hard-coded protected-header set at CONFIG-LOAD TIME: a `header_mutation` config attempting to mutate any protected header causes Envoy to refuse to boot with a verbatim error `:-prefixed or host headers may not be modified`. The protected set is exactly:

- All five `:`-prefixed pseudo-headers: `:method`, `:path`, `:authority`, `:scheme`, `:status`.
- The HTTP/1.1 `host` header (case-insensitive: `host`, `Host`, `HOST` all rejected).

Protection scope spans listener-level filter configs, per-route `HeaderMutationPerRoute` configs, `request_mutations`, and `response_mutations` — all four combinations rejected at boot.

envoy-go MIRRORS this discipline by validating each `compiledMutationOp.headerName` against the protected set at `New` time (listener-level) and at HCM-build time (per-route, per §6.7 / §12 deferred decision 3); the verbatim error format is `header_mutation: %q is :-prefixed or host; may not be modified`. Boot-time-fail-fast per ADR-0072 — a misconfigured protected-header mutation surfaces as a non-zero exit before the listener accepts traffic.

#### Stats — none emitted (per phase 10 SPEC §11.3)

`envoy.filters.http.header_mutation` emits ZERO stats. The proto has no `stat_prefix` field; no `header_mutation.*` namespace exists in `/stats` or `/stats?format=prometheus`. envoy-go matches: zero stats. The `## Stat-name mapping ### 22-name table` (extended by phase 09) is UNCHANGED in phase 10.

(Cors @ 07.1 also emits zero stats per ADR-0074. The pattern is established: not every HTTP filter is stat-bearing.)

#### Does not yet apply to (per phase 10 deferrals — ADRs 0112, 0113)

- **`mutations.query_parameter_mutations[]`** (KeyValueMutation triple): path-query rewriting deferred per ADR-0112. envoy-go silently parses the field but does not honor it; configured query-parameter mutations are no-ops.
- **Header-value formatter substitution** (`%REQ(:path)%`, `%DOWNSTREAM_REMOTE_ADDRESS%`, `%RESPONSE_CODE%`, etc.): formatter syntax deferred per ADR-0113. envoy-go materializes header values as STATIC strings verbatim; a configured value of `"%REQ(:path)%"` produces the literal 11-byte string on the wire, not the substituted path.
- **Differential testing under H2 streams**: fixture 0012 is HTTP/1.1-only; H2 differential testing of header_mutation is deferred.
- **Cross-filter interaction tests** (header_mutation × cors, header_mutation × fault): fixture 0012 is header_mutation + router only; cross-filter encode-side ordering with sibling encode-mutating filters is deferred to whatever phase lands the second encode-mutating filter.

#### Empirical evidence (verbatim curl excerpts from phase 10 SPEC §11)

```
$ curl -isS http://127.0.0.1:10000/echo  # listener: OVERWRITE x-multi to OVERWRITTEN, then APPEND APPENDED

HTTP/1.1 200 OK
server: envoy
date: Mon, 04 May 2026 14:37:52 GMT
content-type: text/plain
content-length: 245
x-resp-test: backend-original
x-multi: OVERWRITTEN
x-multi: APPENDED
set-cookie: OVERWRITTEN
x-resp-added: added-via-add-if-absent
```

(Multi-value `x-multi`: OVERWRITE collapsed `alpha`/`beta` to single `OVERWRITTEN`, then APPEND added `APPENDED`. Final response carries 2 `x-multi` values per phase 10 §11.4.)

### envoy.filters.http.local_ratelimit

Phase 11 ships `envoy.filters.http.local_ratelimit` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes 5 of 19 top-level fields and silent-ignores the other 14 per the deferral list below + ADR-0040 silent-ignore discipline.

#### Asserted equivalence (per phase 11 SPEC §11)

When `envoy.filters.http.local_ratelimit` is present in `http_filters`, envoy-go MUST emit the same rate-limit decision (allow or 429) as reference Envoy v1.37.2 for the canonical scenarios: basic-allow, basic-rate-limited, refill-after-fill_interval, and per-route wholesale-override. The four stat counter increments (`enabled`, `ok`, `rate_limited`, `enforced`) MUST be equal after each scenario under the lockstep MVP invariant (`enforced == rate_limited` at every step per ADR-0118). Per-route bucket independence MUST hold: listener-level state is NOT touched for per-route requests.

**Consumed fields (5):**

| Proto field | envoy-go behavior |
|---|---|
| `stat_prefix` | Required (PGV non-empty per ADR-0115). Used as the stat-name prefix and the Prometheus tag-extracted label `envoy_local_http_ratelimit_prefix` value. |
| `token_bucket.max_tokens` | Required (PGV `> 0` per shared `TokenBucket` proto). Initial bucket fill = `max_tokens`. |
| `token_bucket.tokens_per_fill` | Optional; default `1` (matches Envoy proto default). PGV `> 0` if explicitly set. |
| `token_bucket.fill_interval` | Required. Filter-internal validation: `≥ 50ms` (matches Envoy v1.37.2 filter-internal check; error string verbatim: `local rate limit token bucket fill timer must be >= 50ms`). NOT a PGV constraint per ADR-0115. |
| `status.code` | Optional; default 429. PGV `[400, 600)` if explicitly set. Status text follows RFC 7231 (`Too Many Requests` for 429). |

#### Token-bucket primitive (per ADR-0116)

Lazy refill on access (Option A); single `sync.Mutex` per bucket; no per-bucket goroutine; `time.Now().UnixNano()` monotonic clock (Go ≥1.9 guarantee). Hot-path: 5–10 nanoseconds typical (compute elapsed → integer-divide by `fill_interval` → conditional add → decrement). LBP-1-adjacent discipline: the lazy-refill approach avoids goroutine proliferation per-bucket under high-cardinality per-route configs.

#### Per-route override semantics (per ADR-0117 + ADR-0073 amendment)

Wholesale-override per ADR-0073 + ADR-0117 (ADR-0073 amendment). Each per-route TPFC entry (`*LocalRateLimit` proto; same type as listener-level) runs through `New` at config-load time, allocating its own `*runtimeConfig` + own `*tokenBucket` + own `*filterStats`. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration) picks the most-specific config per request. Listener-level state is NOT touched for per-route requests (per §11.6 empirical confirmation). This is the FIRST production filter in envoy-go where per-route TPFC override implies independent stateful resources; cors / fault / header_mutation per-route configs are data-only.

#### Rate-limited response wire shape (per ADR-0119 + SPEC §11.3)

- Status: `429 Too Many Requests`
- Body: `local_rate_limited` (18 bytes ASCII; NO trailing newline)
- Headers in lexicographic order: `content-length: 18`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Framing: Content-Length

The `SendLocalReply` mechanism mirrors fault abort (per ADR-0102 + ADR-0103). The 4-header set is lowercase wire-form; `server: envoy` (NOT `envoy-go`) per empirical pin §11.3.

#### Allow-path response (per SPEC §11.8)

NO `x-ratelimit-*` headers added by the filter on either side (request or response). `enable_x_ratelimit_headers` is a silent-ignored field in phase 11 (see § Silent-ignored fields below). Standard HCM/router headers (`server`, `date`, `x-envoy-*`) are unrelated to this filter.

#### MVP invariant (per ADR-0118)

`enforced == rate_limited` at every step. The `enforced` counter increments lockstep with `rate_limited` under the phase 11 MVP. Future shadow-mode phase widens to `enforced ≤ rate_limited` when `filter_enforced` runtime-key support lands.

#### Stats (per SPEC §11.5 + Rule SN9)

Four counters per `stat_prefix`: `http_local_rate_limit.enabled`, `http_local_rate_limit.ok`, `http_local_rate_limit.rate_limited`, `http_local_rate_limit.enforced`. A filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` is registered to extract the `<stat_prefix>` segment from the stat name. See `## Stat-name mapping ### 26-name table` for the full table.

#### Silent-ignored fields (14, organized by family)

- *Descriptor-action* (4): `descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors` — couples to `global_ratelimit` future phase.
- *Runtime + shadow-mode* (3): `filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced` — couples to Runtime + hot restart family. **Note:** reference Envoy defaults `filter_enabled` + `filter_enforced` to 0% (off); fixture configs MUST set both to 100% explicitly for differential equivalence. envoy-go's silent-ignore is equivalent to "always-100%" — divergence-window applies if user omits these fields outside the fixture context.
- *xDS cluster-state* (1): `local_cluster_rate_limit` — couples to xDS / dynamic config family.
- *Response-side header injection* (1): `response_headers_to_add`.
- *Per-connection lifecycle* (1): `local_rate_limit_per_downstream_connection`.
- *Multi-stage limiting* (1): `stage` — couples to descriptor-action subsystem.
- *X-RateLimit headers + vh policy* (2): `enable_x_ratelimit_headers`, `vh_rate_limits`.
- *gRPC trailer mapping* (1): `rate_limited_as_resource_exhausted` — couples to gRPC family.

#### Empirical evidence (rate-limited wire shape from phase 11 SPEC §11.3)

```
$ curl -isS http://127.0.0.1:10000/  # after bucket exhausted (tokens=0)

HTTP/1.1 429 Too Many Requests
content-length: 18
content-type: text/plain
date: <RFC1123>
server: envoy

local_rate_limited
```

(18-byte body `local_rate_limited`, no trailing newline. 4-header set in lexicographic order. Content-Length framing.)

### envoy.filters.http.csrf

Phase 12 ships `envoy.filters.http.csrf` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes 1 of 3 top-level fields actively, validates 1 at parse-time but silent-ignores its runtime value, and silent-ignores 1 entirely.

**Field decomposition (3 fields):**

| Proto field | envoy-go behavior |
|---|---|
| `additional_origins` | CONSUMED. Repeated `StringMatcher`. Only `exact` variant with non-empty value is honored; non-exact variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) are dropped at PARSE time per ADR-0101 §3 discipline. Empty-value `exact` entries are also dropped. |
| `filter_enabled` | PGV-VALIDATED at parse-time (per phase 12 SPEC §11.11): envoy-go's `New` factory rejects with a non-nil error if the field is nil OR if its inner `default_value` is nil — mirroring Envoy's PGV envelope. SILENT-IGNORED at runtime: the percentage value is read but not consulted; the filter always evaluates as if 100%-active. Couples to deferred Runtime + hot restart family. |
| `shadow_enabled` | OPTIONAL at parse-time (Envoy permits omission); SILENT-IGNORED at runtime; always-never-shadow regardless of proto value. Couples to deferred Runtime + hot restart family. |

**Method gate:** csrf evaluates only modifying-method requests `{POST, PUT, DELETE, PATCH}` (case-sensitive uppercase string match against `:method`). Non-modifying methods (`GET`, `HEAD`, `OPTIONS`, `TRACE`, `CONNECT`, `PROPFIND`, custom verbs) short-circuit to `Continue` BEFORE any state touch — no counter increment, no origin parse. CONNECT may be rejected at the HCM level (400 Bad Request) before csrf is reached; this is unrelated to csrf. (Per phase 12 SPEC §11.1.)

**Origin extraction trichotomy** (per phase 12 SPEC §11.2):

1. `Origin: null` (literal 4-char string `"null"`) → empty source_origin → `missing_source_origin` counter increment → reject. NO Referer fallback.
2. `Origin:` empty value OR `Origin:` header absent → empty `hostAndPort()` → fall back to Referer's `hostAndPort()`. If Referer also yields empty `hostAndPort()` → empty source_origin → `missing_source_origin` → reject.
3. `Origin:` non-empty, non-`null`, BUT URL parse fails (e.g., `Origin: not-a-url`) → return the verbatim raw string as source_origin. NO Referer fallback. The verbatim string almost always rejects (since it mismatches the target's `hostAndPort` and any `additional_origins[].exact` entry — unless an entry happens to equal that exact verbatim string).

**Comparison algorithm — HOST:PORT-ONLY equality** (per phase 12 SPEC §11.3 / §11.7 / §11.8):

- Source `hostAndPort` is computed via URL parse of the `Origin:` (or `Referer:`) value; if parse succeeds, the result is `host[:port]`. If parse fails, the verbatim raw string is used.
- Target `hostAndPort` is computed via URL parse of `<scheme>://<:authority>`, where `<scheme>` is the request's `:scheme` pseudo-header (set by HCM from listener TLS state) and `<:authority>` is the `:authority` pseudo-header (HTTP/2) or `Host` header (HTTP/1.1). The scheme is consumed only to make the URL parseable; `hostAndPort()` strips it.
- Equality is byte-exact between the two `host[:port]` strings.
- **NO case folding.** `https://APP.EXAMPLE.TEST` does NOT match `app.example.test`. Operators MUST author configs in the lowercase form they expect Origin headers to carry.
- **NO default-port stripping.** `https://app.example.test:443` does NOT match `app.example.test`. To support implicit-default-port equivalence, operators must explicitly add both port-suffixed and bare entries to `additional_origins`.
- **Trailing slash IS stripped** (path component dropped via URL parser). `https://app.example.test/` yields `hostAndPort = app.example.test`.
- **`X-Forwarded-Proto` is irrelevant.** Scheme is computed only for URL parsing; its value is stripped before equality.

**Operator footgun (per phase 12 SPEC §11.7 + §11.8):** `additional_origins[].exact` is matched against the source's `host[:port]` form — NOT the full URL with scheme. Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header, because the source's `hostAndPort` is `app.example.test` (not `https://app.example.test`). Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port); DO NOT include the scheme prefix. envoy-go faithfully replicates Envoy's behavior; this is a known footgun in the upstream spec.

**Per-route override semantics:** Wholesale-override per ADR-0073. Each `CsrfPolicy` TPFC entry runs through `New` at config-load time, allocating its own `*runtimeConfig` with its own compiled `[]additionalOrigins` slice. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration) picks the most-specific config per request. Listener-level data is NOT touched for per-route reqs.

**Per-route stats are SHARED with listener-level** (per phase 12 SPEC §11.9; **diverges from phase 11 local_ratelimit precedent**). The per-route `runtimeConfig` carries only the `additional_origins` data; stat counter increments always go to the listener-level `*filterStats` registered for the HCM scope. There is exactly ONE counter series per HCM stat_prefix per counter, regardless of how many per-route TPFC entries exist. Confirmed empirically: 4 requests across listener-level (`/`) and per-route (`/route`) increment the counters as a SUM (e.g., 2 valid + 2 invalid total, NOT split into separate series).

**Rejection response wire shape (per phase 12 SPEC §11.10 empirical):**

- Status: `403 Forbidden`
- Body: `Invalid origin` (14 bytes ASCII; NO trailing newline)
- Headers in lexicographic order: `content-length: 14`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Framing: Content-Length (NO chunked)
- NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding`, NO `charset=UTF-8` modifier on `content-type`

**Allow-path response (per phase 12 SPEC §11.6 empirical):** NO csrf-specific headers added on either side (request or response). Standard HCM/router headers (`server`, `date`, `x-envoy-*`) are unrelated to this filter.

**Stat surface (3 counters per HCM scope):**

- `http.<HCM stat_prefix>.csrf.request_valid` — incremented when modifying-method request's source origin matches target OR any `additional_origins[].exact`.
- `http.<HCM stat_prefix>.csrf.request_invalid` — incremented when source origin is determinable but matches neither.
- `http.<HCM stat_prefix>.csrf.missing_source_origin` — incremented when source origin is undeterminable (Origin: null literal, OR both Origin and Referer missing/yield-empty-hostAndPort).

NO `shadow_request_invalid` counter — confirmed reference Envoy v1.37.2 also does not emit it under all-defaults config (shadow-only mode reuses the regular 3-counter family).

### envoy.filters.http.buffer

Phase 13 ships `envoy.filters.http.buffer` per the canonical Envoy v1.37.2 filter spec. envoy-go consumes the only top-level field on the parent `Buffer` proto actively, with envoy-go-own validation (≤ 1 MiB ceiling) that closes a divergence-window against reference Envoy's lack of such a ceiling.

**Listener-level field decomposition (1 field):**

| Proto field | envoy-go behavior |
|---|---|
| `max_request_bytes` (`UInt32Value`, REQUIRED) | CONSUMED. envoy-go-own validation: non-nil + value > 0 + value ≤ 1048576 (1 MiB). Rejected at parse time with envoy-go-own error wording. The 1 MiB ceiling is the load-bearing envoy-go-only divergence (reference Envoy accepts arbitrary `UInt32Value`). |

**Per-route TPFC (`BufferPerRoute` proto — separate from listener-level `Buffer`):**

| Proto field | envoy-go behavior |
|---|---|
| `override.disabled` (oneof case, `bool`, PGV `bool.const = true`) | CONSUMED. `disabled: true` → filter wholly inactive on this route. `disabled: false` rejected at parse-time per PGV mirror. |
| `override.buffer` (oneof case, `Buffer` message) | CONSUMED. `buffer.max_request_bytes` subjected to the same ≤ 1 MiB validation as listener-level. Wholesale-override of listener cap per ADR-0073. |

The `oneof override` carries PGV `validate.required = true` — exactly one case must be set; both-set + neither-set rejected at boot via the JSON→proto decoder (NOT PGV; mechanism per phase 13 SPEC §11.3 + ADR-0125).

**Body-counting algorithm — STREAMING-CAP ONLY (per phase 13 SPEC §11.6):**

The buffer filter does NOT inspect `Content-Length` in `DecodeHeaders`. The cap fires only after data accumulates past the limit:

1. `DecodeHeaders(headers, endStream)`:
   - `endStream=true` (header-only) → `Continue` (no body work; mirrors `buffer_filter.cc:54-56`).
   - per-route resolves to `disabled=true` → set passthrough flag; `Continue` (mirrors `buffer_filter.cc:60-62`).
   - bodied + non-disabled → store `effectiveMax` + `headersRef`; return `Continue`. (envoy-go's HCM dispatches headers + body sequentially in one goroutine; `StopIteration` here would deadlock — see ADR-0128 for the synchronous-HCM dispatch constraint. Observable behavior is identical to reference Envoy's `StopIteration` path: headers are held until the body counting completes.)
2. `DecodeData(data, endStream)`:
   - passthrough flag set → `DataContinue` (filter never returns `DataStopIterationAndBuffer` on this path; framework safety-net cap never engages on disabled routes).
   - `accumulated += len(data)`.
   - `accumulated > effectiveMax` → `SendLocalReply(413, "Payload Too Large", connClose)` + `DataStopIterationNoBuffer` (discards partial buffer).
   - `endStream=true` (terminal chunk fits) → invoke `maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`); release held headers + body; `DataContinue`.
   - in-flight chunk → `DataContinue` (envoy-go's HCM already accumulates all body bytes in `connection.go`'s `bodyBuf` before the router action dials upstream per ADR-0076 + ADR-0128; `DataStopIterationAndBuffer` is not needed — the framework holds the bytes. Observable behavior identical to reference Envoy's `DataStopIterationAndBuffer` path.)
3. `DecodeTrailers` → invoke `maybeAddContentLength` defensively; `TrailersContinue`.
4. `Encode*` → all pass-through. Buffer is decoder-side-only.
5. `OnDestroy` → no-op.

`maybeAddContentLength` (mirrors `buffer_filter.cc:91-97`): if `headersRef != nil` AND original request had no `Content-Length` → set `Content-Length: <accumulated>` on the held headers AND drop `Transfer-Encoding: chunked`. The discipline is: chunked → fixed-CL conversion before forwarding upstream. Observable at the backend boundary (per phase 13 SPEC §11.8-CL).

**Per-route override semantics:** Wholesale-override per ADR-0073 + ADR-0125 (5th canonical per-route discipline: disabled-OR-override). Each `BufferPerRoute` TPFC entry runs through `parsePerRoute` at config-load time, allocating its own `*compiledPerRoute{disabled, maxOverride}`. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) picks the most-specific config per request.

**Per-route stats are SHARED with listener-level** (mirrors phase 12 csrf ADR-0124; DIVERGES from phase 11 local_ratelimit ADR-0117). Phase 13 emits NO filter-specific counters (see "Stat surface" below); the SHARED-stats invariant is structurally vacuous for buffer (no counters to share or split) but documented for cross-filter consistency.

**Cap layering against ADR-0076 framework safety net:** The buffer filter's `accumulated > effectiveMax` check fires INSIDE the framework's hardcoded `filterBufferLimitBytes = 1 << 20` cap (per `internal/filter/http/chain.go:19`). Because `effectiveMax ≤ 1 MiB` (invariant per ADR-0126's parse-time validation), the framework cap is structurally unreachable in MVP — the filter wins by construction. The framework cap remains armed as a safety net for any future configuration that might bypass the parse-time check (e.g., the future cap-promotion phase that promotes `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes` from silent-ignored to honored).

**Rejection response wire shape (per phase 13 SPEC §11.7 + §11.8 — REUSES framework path verbatim per ADR-0076 §Decision (b)):**

- Status: `413 Payload Too Large`
- Body: `Payload Too Large` (17 bytes ASCII; constant `localReply413Body` from `internal/filter/http/chain.go:25`; NO trailing newline)
- Headers in lexicographic order: `content-length: 17`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`
- Plus user-supplied `Connection: close`
- Framing: Content-Length (NO chunked)
- NO `cache-control`, NO `x-content-type-options`, NO `transfer-encoding`, NO `charset=UTF-8` modifier on `content-type`

**100-Continue note:** envoy-go's `connection.go:122` categorically rejects any request carrying `Expect:` with a 417 response (pre-filter-chain guard; see phase 04). The `Expect: 100-continue` scenario documented in the original ADR-0127 v2 §Decision (v) cannot fire at the buffer-filter level; it is tracked for future fix in the phase-04 Expect-handling bundle (see ADR-0127 v2 §Decision (v) retraction).

**Method gate:** Buffer evaluates ALL methods that carry a body; the method itself is not consulted. Bodied requests (`POST`, `PUT`, `PATCH`, `DELETE` with body, etc.) all pass through the streaming-cap path; non-bodied requests (`GET`, `HEAD`, `OPTIONS` with `endStream=true` on headers) short-circuit at `DecodeHeaders` step 1 (`Continue` without state touch).

**Stat surface:** ZERO filter-specific counters. Reference Envoy v1.37.2 emits NO `envoy_http_buffer_*` counter family at all (confirmed at phase 13 SPEC §11.5 — `/stats/prometheus` scrape after 4 buffer-overflow probes shows ZERO `envoy_http_buffer_*` lines). Buffer-filter overflow is observable on envoy-go's side via the existing HCM `downstream_rq_4xx` counter (in the 29-name table per phase 06.1; rendered as `envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="<HCM stat_prefix>"}`). The Envoy-only `downstream_rq_too_large` and `downstream_rq_completed` HCM counters increment alongside in reference Envoy but are NOT in envoy-go's emit allow-list — they are filtered out per the `### Twin-series filter discipline` allow-list discipline (above).

### Empirical evidence (413 overflow)

**Probe configuration:** chain `[envoy.filters.http.buffer, envoy.filters.http.router]` with `Buffer{max_request_bytes: 1024}`. Listener `per_connection_buffer_limit_bytes: 1024`. Route `/` → STRICT_DNS cluster `c0` (nginx backend).

**Probe request:** `POST / HTTP/1.1` `Content-Length: 2048` with a 2048-byte body of ASCII `'A'`.

**Verbatim response:**

```
< HTTP/1.1 413 Payload Too Large
< content-length: 17
< content-type: text/plain
< date: Fri, 01 May 2026 01:10:15 GMT
< server: envoy
< connection: close
```

**Body bytes (verbatim hex dump):**

```
00000000: 5061 796c 6f61 6420 546f 6f20 4c61 7267  Payload Too Larg
00000010: 65                                       e
```

**Conclusions (pinned):**

- Status: `413 Payload Too Large`.
- Body: 17 bytes, exact ASCII `Payload Too Large` (no trailing newline).
- Headers in wire order: `content-length: 17`, `content-type: text/plain`, `date: <stamp>`, `server: envoy`, `connection: close`.
- Connection is closed (note `connection: close`) — the 413 forces the H1 conn to terminate; envoy-go's encode-side overflow must mirror this discipline (H1: emit 413 then close conn; H2: RST_STREAM after the local-reply HEADERS+DATA frames). The `connection: close` header is what makes the 413 path's connection-reset semantically explicit on the wire.
- envoy-go's decode-side buffer overflow MUST synthesize this verbatim shape (status + body + headers — modulo `date` and `server` which are already in the differential allow-list per `BEHAVIOR_CONTRACT.md ## Header allow-list`).

### Empirical evidence (sendLocalReply entry)

**Probe configuration:** chain `[lua_a, lua_b, lua_c, envoy.filters.http.router]` where `lua_b` calls `respond` (Envoy's sendLocalReply API) with status 418 + a `x-from: filterB` header + body `"418 from filterB\n"`. `lua_a`, `lua_c` log on decode/encode entry. Route `/` → STRICT_DNS cluster `c0` (would route to nginx, but `lua_b`'s `respond` aborts decode mid-chain).

**Probe request:** `GET / HTTP/1.1`

**Verbatim Envoy stderr trace (timestamps preserved):**

```
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=A index=0
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: DECODE filter=B index=1 (calling respond)
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=C index=2
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=B index=1
[2026-05-01 01:11:17.263][13][critical][lua] [source/extensions/filters/common/lua/lua.cc:35] script log: ENCODE filter=A index=0
```

**Verbatim response:**

```
< HTTP/1.1 418 Unknown
< x-from: filterB
< content-length: 17
< content-type: text/plain
< date: Fri, 01 May 2026 01:11:16 GMT
< server: envoy
```

**Conclusions (pinned):**

- Decode aborted at `lua_b` (index 1) when it called `respond`. `lua_c` (index 2) and router (index 3) NEVER ran on the decode side.
- Encode-side iteration entered at **`lua_c` (index 2)** — i.e., `filter[len-1]` of the encode-side filter set (router has no observable encode-side action in this probe but is at index 3; the encode-side iteration starts from the last filter that has an encode side, which in this chain is `lua_c` at index 2).
- ALL THREE Lua filters' encode sides ran (`lua_c` → `lua_b` → `lua_a`), even though only filter B's decode side reached its abort point — confirming that `sendLocalReply` runs the FULL encode chain in reverse order, not just from the abort point upward.
- Status `418 Unknown`: HTTP/1.1 status text "Unknown" because 418 is not a stdlib-known status code on Envoy's HTTP/1.1 codec; the payload includes the user-supplied `x-from: filterB` header alongside the framework-injected `content-length` / `content-type` / `date` / `server`.
- envoy-go's `chain.beginLocalReply` MUST: (a) abort decode-side iteration at the calling filter's index; (b) enter encode-side iteration at `filter[len-1]` of the encode-side set (NOT at the calling filter's index, NOT at index 0); (c) iterate the FULL encode chain in reverse order (every encode-side filter runs); (d) merge framework-injected standard headers (`content-length`, `content-type`, `date`, `server`) with the user-supplied headers (`x-from`).

### envoy.filters.http.compressor

Phase 14 ships `envoy.filters.http.compressor` (gzip-only response-side MVP) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the SEVENTH `§9` family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13). It is the SECOND filter using the ADR-0125 5th canonical disabled-OR-override per-route discipline (after buffer).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.compressor.v3.Compressor` (15 fields enumerated; 6 consumed + 9 silent-ignored at the listener-level proto):**

| Field | Type | Phase 14 disposition | Notes |
|---|---|---|---|
| `compressor_library` | TypedExtensionConfig | CONSUMED | REQUIRED per Envoy PGV; envoy-go gzip-only MVP rejects non-Gzip TypeURLs at parse with envoy-go-own error (per ADR-0130). |
| `response_direction_config.common_config.min_content_length` | UInt32Value | CONSUMED | Default 30 (per phase 14 SPEC §11.9 empirical). |
| `response_direction_config.common_config.content_type` | []string | CONSUMED | Default 8-entry list (per phase 14 SPEC §11.1 empirical). |
| `response_direction_config.disable_on_etag_header` | bool | CONSUMED | Dual-mode per phase 14 SPEC §1.1 amendment 6 + §11.7. |
| `response_direction_config.remove_accept_encoding_header` | bool | CONSUMED | Strips Accept-Encoding from upstream-bound request. |
| `response_direction_config.uncompressible_response_codes` | []uint32 | CONSUMED | Default empty `[]` (per phase 14 SPEC §11.2 empirical). |
| `response_direction_config.common_config.enabled` | RuntimeFeatureFlag (BoolValue default) | SILENT-IGNORED | Always-active runtime; OPTIONAL at parse-time (per phase 14 SPEC §1.1 amendment 2 + §11.3). Divergence-window if `default_value: false`. |
| `response_direction_config.common_config.runtime_key` | string | SILENT-IGNORED | Couples to Runtime + hot restart family. |
| `response_direction_config.response_direction_feature_*` | various | SILENT-IGNORED | Subset of compressor-specific knobs not exposed in MVP. |
| `status_header_enabled` | bool | SILENT-IGNORED | Always-no-status-header; the `x-envoy-compression-status:` debug header is not emitted. Divergence-window if set true. |
| `request_direction_config` | RequestDirectionConfig | SILENT-IGNORED | Always-disabled; envoy-go MVP is response-only. Divergence-window if set with `enabled: true`. |
| `runtime_enabled` | RuntimeFeatureFlag | SILENT-IGNORED | Deprecated; superseded by `response_direction_config.common_config.enabled`. |
| `choose_first` | bool | SILENT-IGNORED | Always-q-value-based selection. Divergence-window if `true` AND multi-coding AE. |
| `content_length` (deprecated) | UInt32Value | SILENT-IGNORED | Deprecated top-level mirror of `response_direction_config.common_config.min_content_length`. |
| `content_type` (deprecated) | []string | SILENT-IGNORED | Deprecated top-level mirror. |
| `disable_on_etag_header` (deprecated) | bool | SILENT-IGNORED | Deprecated top-level mirror. |
| `remove_accept_encoding_header` (deprecated) | bool | SILENT-IGNORED | Deprecated top-level mirror. |

**Codec-library `envoy.extensions.compression.gzip.compressor.v3.Gzip` (5 fields; 2 consumed + 3 silent-ignored):**

| Field | Type | Phase 14 disposition | Notes |
|---|---|---|---|
| `compression_level` | enum | CONSUMED | Mapped to Go `compress/gzip` level constant per ADR-0130 §Decision (iv) table. |
| `compression_strategy` | enum | CONSUMED | Only `HUFFMAN_ONLY` honored; all others collapse to default. |
| `memory_level` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz memory-level knob. |
| `window_bits` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz window-bits knob. |
| `chunk_size` | UInt32Value | SILENT-IGNORED | Go gzip does not expose libz chunk-size knob. |

`compressor_library` non-Gzip TypeURLs are PARSE-REJECTED at boot with envoy-go-own error wording (per ADR-0130). Reference Envoy accepts `envoy.extensions.compression.{brotli,zstd}.compressor.v3.{Brotli,Zstd}`; envoy-go MVP refuses.

**Per-route `CompressorPerRoute`:** oneof `disabled: true` OR `overrides: CompressorOverrides`. The `CompressorOverrides` shape carries `response_direction_config: ResponseDirectionOverrides` (only `remove_accept_encoding_header` BoolValue) + `compressor_library: TypedExtensionConfig` (silent-ignored per phase 14 SPEC §2.2). Per-route override of `min_content_length`, `content_type`, `disable_on_etag_header`, `uncompressible_response_codes`, `enabled`, `status_header_enabled`, `runtime_enabled`, `choose_first`, `request_direction_config` is STRUCTURALLY IMPOSSIBLE in Envoy v1.37.2's proto.

#### Wire shape

Compressed-path response wire shape (envoy-go MVP):
- `content-type: <preserved>` — content-type preserved from upstream/direct_response.
- `content-encoding: gzip` — set by filter on compress path.
- `vary: Accept-Encoding` — appended to existing Vary OR set if absent (per phase 14 SPEC §1.1 amendment 5 — APPEND ALWAYS, even on existing `Vary: *`). Token-match dedup (case-insensitive).
- `content-length: <gzipped-byte-count>` — fixed Content-Length (envoy-go writeH1Reply unconditional rewrite per `internal/filter/hcm/codec.go:87-89`).
- NO `transfer-encoding: chunked` — envoy-go MVP does not support chunked output on the encode side.
- ETag mode-a (`disable_on_etag_header: false`, default): strong-ETag stripped (regex `^"[^"]*"$` match → header deleted); weak-ETag preserved (regex `^W/"[^"]*"$`). RFC 7232 §2.3 motivation per phase 14 SPEC §1.1 amendment 6.
- ETag mode-b (`disable_on_etag_header: true`): skip compression entirely on any ETag presence; `not_compressed_etag +1`.

**Wire-shape divergence-window from reference Envoy (deliberate; ADR-0131 records):** Envoy emits `transfer-encoding: chunked` (NO `content-length`) on every compressed response (per phase 14 SPEC §11.9 empirical evidence covering body sizes 30 / 1024 bytes). envoy-go MVP emits fixed CL identity. The differential fixture 0016's allow-list excludes `content-length` value comparison + `transfer-encoding` presence on compressed scenarios.

Compressed-path body shape: **decompressed-byte-equivalent to original** (per phase 14 SPEC §11.14 + ADR-0133 — decompress-and-compare assertion); compressed bytes structurally non-byte-exact between Go `compress/gzip` (default `OS: 255`, varying `XFL`) and Envoy libz (`OS: 03 UNIX`, `XFL: 00`). Forward-pointer to a future encode-side streaming framework phase per ADR-0131.

Skip-path response wire shape: identity body unmodified; NO `content-encoding`; Vary INJECTED on AE-side-skip paths (no AE / identity / wildcard-uncompressed / not_valid) but NOT on server-side-skip paths (content-type-mismatch / already-encoded / etag-disabled / no-transform / content-length-too-small / uncompressible-status) per phase 14 SPEC §1.1 amendment / §11.15.

#### Per-route disabled-OR-override 5th canonical (per ADR-0125 amendment §(viii)-(x) + phase 14 SPEC §1.1 amendment 4)

Phase 14 is the SECOND row using ADR-0125 5th canonical disabled-OR-override discipline (after phase-13 buffer). Per-route override surface is FILTER-SPECIFIC and NARROWER than the listener-level config (only `remove_accept_encoding_header` per `ResponseDirectionOverrides` proto + per-route library swap silent-ignored). Per-route stats SHARED with listener-level (mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125; DIVERGES from phase-11 local_ratelimit ADR-0117 INDEPENDENT-stats).

#### HCM `directResponseAction.response_headers_to_add` plumbing (per ADR-0134)

Phase 14 introduces a single new framework primitive at the HCM `directResponseAction` boundary: route-level `response_headers_to_add` entries are now plumbed through `buildExtraResponseHeaders` into `directResponseAction.extraHeaders` and applied at `body()` with `OVERWRITE_IF_EXISTS_OR_ADD` semantics. The motivation is fixture-0016 scenario 2 (image/png content-type-skip path), which requires the route's `Content-Type: image/png` override to take effect on envoy-go's direct_response body so the compressor's content-type predicate sees the correct value. Only the `OVERWRITE_IF_EXISTS_OR_ADD` `AppendAction` is honored at parse; `APPEND_IF_EXISTS_OR_ADD` / `ADD_IF_ABSENT` / `OVERWRITE_IF_EXISTS` are reserved for future support (parse-time rejection). Fixture configs MUST set the action explicitly. See ADR-0134.

#### Stat surface (17 counters per HCM stat_prefix; per ADR-0132)

Phase 14 emits 17 counters per HCM stat_prefix, namespaced `compressor.<library_name>.<codec>.[response.]<counter>` per phase 14 SPEC §11.5 empirical scrape. The 17-counter set is enumerated in `## Stat-name mapping ### 46-name table` below. Per-route stats are SHARED with listener-level (mirror of phase-12 csrf + phase-13 buffer; DIVERGES from phase-11 local_ratelimit independent stats per ADR-0117).

**Per-counter empirical divergence-window pinned at Task 14** (4 counters where reference Envoy v1.37.2 + envoy-go diverge by design choice; both sides are valid implementations of the contract — see Phase 14 forward-pointer notes for the table):

- `header_compressor_used` (ref=3, subj=5 on the 0016 6-scenario workload) — envoy-go caches Accept-Encoding classification BEFORE per-route `remove_accept_encoding_header` strips the header (per ADR-0129 same-`*filter` discipline); reference Envoy reclassifies post-strip.
- `header_not_valid` (ref=1, subj=0) — reference Envoy's post-strip reclassification on per-route-rmAE routes classifies as `not_valid`; envoy-go's cached state classifies as `no_accept_header`.
- `response_not_compressed` (ref=3, subj=2) — reference Envoy's per-route-disabled scenario STILL increments the counter despite the filter being wholly inactive; envoy-go's per-route-disabled path is wholly silent per ADR-0125.
- `request_not_compressed` (ref=6, subj=0) — reference Envoy increments PER REQUEST even with `response_direction_config`-only setups; envoy-go MVP's request side is silent per ADR-0132 twin-series discipline (couples to future decompressor phase).

These four counters are tested via per-side empirical assertion in fixture 0016 (mode `counterModePerSideExact`); cross-side delta-equality applies to the other 13 counters. See Phase 14 forward-pointer notes for forward-points.

### envoy.filters.http.bandwidth_limit

Phase 15 ships `envoy.filters.http.bandwidth_limit` (Envoy v1.37.2 canonical symmetric request+response throttle filter; **limit in KiB/s — kibibytes-per-second per proto comment at `bandwidth_limit.pb.go:95` + phase 15 SPEC §1.1 amendment 6; NOT kilobits-per-second as BRAINSTORM hypothesized**) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the EIGHTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14). It is the FIRST §9 family-row since phase-12 csrf to introduce **ZERO framework deltas** — composing wholesale against phase-09 fault `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume, phase-13 ADR-0128 decode-side body-buffering machinery, and phase-14 ADR-0131 encode-side `OverwriteBody` primitive (anticipated: NOT invoked — the framework's buffered-return path returns bytes unchanged via `ContinueEncoding`). It is the SECOND row using the 4th canonical stateful-override-with-INDEPENDENT-stats per-route discipline (ADR-0117 codifies; phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND); ADR-0125 §(xi) in-place amendment documents the NEW canonical "bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route" 6th canonical pattern adjacent to ADR-0117.

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.bandwidth_limit.v3.BandwidthLimit` (7 top-level fields total per `[#next-free-field: 8]`):**

| Field | Type | Phase 15 disposition | Notes |
|---|---|---|---|
| `stat_prefix` | string | CONSUMED | REQUIRED per Envoy PGV `min_len = 1` (per phase 15 SPEC §11.P2 + §1.1 amendment 3); envoy-go-mirror PGV-validation at parse time. |
| `enable_mode` | enum (4 values) | CONSUMED | All 4 values honored: DISABLED, REQUEST, RESPONSE, REQUEST_AND_RESPONSE (per phase 15 SPEC §2.2 + §11.P12). |
| `limit_kbps` | UInt64Value (**KiB/s** per proto comment + phase 15 SPEC §1.1 amendment 6) | CONSUMED | OPTIONAL at listener-level (FOOT-GUN: filter loads but request HANGS at runtime if unset + `enable_mode != DISABLED`; per phase 15 SPEC §1.1 amendment 10 + §11 probeJ); CODE-LEVEL REQUIRED at per-route per `source/extensions/filters/http/bandwidth_limit/config.cc::createRouteSpecificFilterConfigTyped` (per phase 15 SPEC §11.P1 verbatim "limit must be set for per route filter config"); PGV `gte = 1` when wrapper present (per phase 15 SPEC §1.1 amendment 4). |
| `fill_interval` | google.protobuf.Duration | CONSUMED | OPTIONAL; default 50ms; PGV `gte = 20ms, lte = 1s` when wrapper present (per phase 15 SPEC §1.1 amendment 5 + §11.P5). GOVERNS chunk-size: `chunk_size = limit_kbps × 1024 × fill_interval_seconds` bytes (per phase 15 SPEC §11.P15). |
| `runtime_enabled` | RuntimeFeatureFlag | SILENT-IGNORED | Always-active runtime; envoy-go always-100%-active regardless of `default_value`. Divergence-window if `default_value: false`. |
| `enable_response_trailers` | bool | SILENT-IGNORED | Always-no-trailers in envoy-go MVP; trailer-emission primitive deferred. Divergence-window if set true (Envoy emits 4 `<prefix>bandwidth-{request,response}{,-filter}-delay-ms` trailers). |
| `response_trailer_prefix` | string | SILENT-IGNORED | Couples to `enable_response_trailers`; PGV pattern `^[^\x00\n\r]*$` enforced at parse-time. |

**Per-route `BandwidthLimit`:** SAME proto as listener-level (NO `BandwidthLimitPerRoute` wrapper per phase 15 SPEC §11.P1). Per-route `limit_kbps` is FILTER-INTERNAL REQUIRED via code-level extra check (per filter source); `stat_prefix` is PGV REQUIRED regardless of position. Per-route `enable_mode: DISABLED` is the canonical disable-on-route mechanism (NO `disabled` boolean shortcut at the proto level). Per ADR-0117 + ADR-0139 + phase 15 SPEC §5: per-route stats are INDEPENDENT (per-route override allocates own `*compiledConfig` + own `*filterStats` via `sync.Map` lazy-cache keyed by `*BandwidthLimit` pointer).

#### Wire shape

Throttled-path wire shape (envoy-go MVP, Path B-async per ADR-0137 + phase 15 SPEC §1.1 amendment 6):
- Response headers: preserved verbatim from upstream/direct_response (no header mutation by bandwidth_limit).
- Response body: byte-equivalent to original (bandwidth_limit does NOT transform bytes).
- Response timing: ONE-BLAST emission at the end of the throttle window. Specifically: `DecodeData(endStream=true)` buffers + computes `throttle = ceil(body_size / chunk_size) × fill_interval` (where `chunk_size = limit_kbps × 1024 × fill_interval_seconds`) + arms `time.AfterFunc(throttle, ...)`; timer fires → `ContinueDecoding` resumes the chain → buffered body forwards upstream in one shot. Symmetric for encode-side via `ContinueEncoding`.

**Decode-data non-endStream `DataContinue` discipline (envoy-go HCM synchronous-dispatch deadlock avoidance; per Task-14 impl-time finding (d) + phase-13 buffer precedent):** envoy-go's HCM dispatches filter callbacks synchronously on the request-handling goroutine. Returning `DataStopIterationAndBuffer` from a non-endStream `DecodeData` invocation BLOCKS the dispatch loop awaiting `ContinueDecoding`, but `ContinueDecoding` is invoked from within that same goroutine — deadlock. Phase 15 mirrors phase-13 buffer's analogous discipline: non-endStream `DecodeData` returns `DataContinue` and accumulates the chunk locally onto `f.requestBody`; ONLY the endStream invocation returns `DataStopIterationAndBuffer` + arms the timer. The framework's buffered-return path returns the accumulated bytes through `ContinueDecoding/Encoding` without explicit replace. ADR-0137 documents the algorithmic divergence from the SPEC §6.5 verbatim algorithm; the wire outcome is byte-equivalent.

**Wire-shape divergence-window from reference Envoy (deliberate; ADR-0137 records; per phase 15 SPEC §11.P8):** Envoy emits Path A rate-paced chunks AT exact `fill_interval` CADENCE — `chunk_size` bytes per tick, distributed across the throttle window (e.g., 77 chunks at 51-byte cadence for `body=4000, kbps=1, fill=50ms` per probeL; 8 chunks at 512-byte cadence for `body=4000, kbps=10, fill=50ms`). envoy-go MVP emits zero chunks during the throttle window, then ALL bytes in one blast at the end. Total wall-clock throttle time is observably equivalent within **±70ms per-side** tolerance (per phase 15 SPEC §11.P9 + Task-14 per-side discipline adoption — see `## Timing tolerances` extension). For consumers that don't depend on intra-throttle chunk timing (typical HTTP clients), the byte-stream is delivered with the same total latency budget.

Forward-pointer per ADR-0137: a future encode-side streaming framework phase lands `EncoderFilterCallbacks.EmitChunk(b []byte)` + symmetric `DecoderFilterCallbacks.ConsumeChunk(b []byte)` + HCM chunk-by-chunk `RunEncodeData/RunDecodeData` invocation. Phase-15 Path B-async naturally upgrades to Path A streaming when those primitives land.

#### Per-route INDEPENDENT-stats discipline (per ADR-0139 + phase 15 SPEC §5)

Phase 15 is the SECOND row using the 4th canonical stateful-override-with-INDEPENDENT-stats per-route discipline (ADR-0117 codifies; phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND). Per-route TPFC entries (same `BandwidthLimit` proto via pointer-identity per-route lazy-cache; bare-message-via-TPFC; phase-15 introduces this as a NEW canonical pattern documented at ADR-0125 §(xi) amendment) own fresh `*compiledConfig` + fresh `*filterStats` keyed by the per-route `stat_prefix`. Listener-level counters do NOT increment for per-route-active streams. DIVERGES from phase-12/13/14 SHARED-stats discipline.

#### Stat surface + Prometheus rendering (per phase 15 SPEC §1.1 amendments 7 + 8 + 9)

Internal stat path: `<stat_prefix>.http_bandwidth_limit.<counter>` (underscore infix; NOT HCM-rooted). Prometheus name: `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` (stat_prefix INLINED into base name; NO labels / NO tag-extractor). The 14 active stat names (8 counters + 6 gauges) are enumerated in `## Stat-name mapping ### 60-name table` above. The 2 unconditional Envoy histograms (`request_transfer_duration`, `response_transfer_duration`) are allow-listed via twin-series-filter divergence-window per the `### Twin-series filter discipline` phase-15 extension + `### Phase 15 forward-pointer notes`.

**Per-counter empirical-divergence window pinned at Task 14** (`*_enabled` + `*_enforced` counters where reference Envoy v1.37.2 + envoy-go diverge by design choice on the 0017 6-scenario workload; both sides are valid implementations of the documented contract — see Phase 15 forward-pointer notes for the cause table):

- `request_enabled` + `response_enabled` — tested via `counterModePerSideExact`. Envoy's `request_enabled` bumps per-active-side-regardless-of-body (initial-burst-discount + per-request bump) while envoy-go's deterministic DecodeData-driven bump increments only when bytes actually arrive at the filter; the per-side counts diverge on tiny-body and within-burst-capacity workloads.
- `request_enforced` + `response_enforced` — tested via `counterModePerSideExact`. Envoy's `*_enforced` semantic increments per `fill_interval` tick during throttle (e.g., probeJ shows `response_enforced: 99` for a hung 5-second stream ≈ 100 ticks at 50ms); envoy-go Path B-async bumps `*_enforced += ticks` at stream-completion to maintain cumulative byte-equivalence with reference Envoy.

These four counters are tested per-side; cross-side delta-equality applies to the four `*_incoming_total_size` + `*_allowed_total_size` counters. The 6 gauges (`*_pending`, `*_incoming_size`, `*_allowed_size` × {request, response}) are NOT asserted by the differential (transient/noisy mid-stream observations).

### envoy.filters.http.rbac

Phase 16 ships `envoy.filters.http.rbac` (Envoy v1.37.2 canonical role-based-access-control filter; decode-side dual-engine policy gate) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the NINTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15). It is the FIRST §9 family-row since phase-14 compressor to introduce non-zero framework deltas + the FIRST single phase to introduce TWO new framework primitives: (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor per ADR-0144 (TLS-principal introspection — cross-phase-reusable by future filters jwt_authn / ext_authz / oauth2 / ext_proc); (ii) `internal/matcher/` NEW top-level package per ADR-0142 (generic `xds.type.matcher.v3.Matcher` match-tree evaluator framework primitive — cross-phase-reusable). It is the THIRD row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent (phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND; phase-16 rbac THIRD); ADR-0145 codifies. ADR-0125 §(xii) in-place amendment documents the NEW canonical "wrapper-with-reserved-field-and-single-optional-sub-message; absent-implies-disabled; presence-implies-wholesale-override" 7th canonical per-route pattern; ADR-0147 unanticipated (Task 13 follow-up) lifts the phase-03 TLS-layer `require_client_certificate=true` blanket-rejection scoped to well-formed mTLS configs to enable fixture 0018 scenario 6.

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.rbac.v3.RBAC` (7 top-level fields per [#next-free-field: 8]):**

| Field | Type | Phase 16 disposition | Notes |
|---|---|---|---|
| `rules` | config.rbac.v3.RBAC | CONSUMED | Primary policy engine; UDPA-`field_alias`-grouped with `matcher` under `rules_specifier`; when both set, `rules` wins per `rbac.pb.go:38` proto comment + phase 16 SPEC §1.1 amendment 2. |
| `shadow_rules` | config.rbac.v3.RBAC | CONSUMED | Parallel non-enforcing engine; UDPA-grouped with `shadow_matcher`; when both set, `shadow_rules` wins per `rbac.pb.go:53`. |
| `shadow_rules_stat_prefix` | string | CONSUMED | Stat namespace tag for shadow counters; OPTIONAL (no PGV per phase 16 SPEC §1.1 amendment 3). |
| `matcher` | xds.type.matcher.v3.Matcher | CONSUMED | Alternative match-tree engine via ADR-0142 framework primitive. |
| `shadow_matcher` | xds.type.matcher.v3.Matcher | CONSUMED | Alternative shadow match-tree. |
| `rules_stat_prefix` | string | CONSUMED | Stat namespace tag for primary counters; OPTIONAL (no PGV). |
| `track_per_rule_stats` | bool | CONSUMED | When true, emit per-policy-name counters per matched policy. |

**Inner `config.rbac.v3.RBAC` (rules-engine config; consumed when `rules` or `shadow_rules` set):**

| Field | Type | Phase 16 disposition | Notes |
|---|---|---|---|
| `action` | RBAC_Action enum | CONSUMED | ALLOW=0 / DENY=1 / LOG=2; PGV `defined_only = true`. LOG = always-allow + match-runs + `access_log_hint` metadata silent (divergence-window per phase 16 SPEC §1.1 amendment 5). |
| `policies` | map<string, Policy> | CONSUMED | Lexicographic-order-of-policy-name walk; Policy = permissions OR (≥1; PGV `min_items=1`) + principals OR (≥1) + condition silent-ignored. |
| `audit_logging_options` | RBAC_AuditLoggingOptions | SILENT-IGNORED | `[#not-implemented-hide:]` upstream; Envoy emits nothing regardless. |

**Inside Policy (3 CEL fields silent-ignored per Q7 + phase 16 SPEC §1.1 amendment 6):**

`condition` (Expr) + `checked_condition` (CheckedExpr) + `cel_config` (CelExpressionConfig) all silent-ignored at runtime. Divergence-window.

**Permission MVP Large 11 (11 of 14; per phase 16 SPEC §1.1 amendment 1 + §2.3):** any, header, url_path, destination_ip, destination_port, destination_port_range, requested_server_name, and_rules, or_rules, not_rule, sourced_metadata (always-no-match runtime).

**Permission DEFERRED (3 of 14):** metadata (deprecated; PARSE-REJECT), matcher (extension; PARSE-REJECT), uri_template (extension; PARSE-REJECT).

**Principal MVP Large 11 (11 of 14 per phase 16 SPEC §1.1 amendment 7 — Principal has 14 variants not 13):** any, authenticated (3-case algorithm per phase 16 SPEC §1.1 amendment 12 + ADR-0144), direct_remote_ip, remote_ip, header, url_path, and_ids, or_ids, not_id, sourced_metadata, filter_state (last two always-no-match runtime).

**Principal DEFERRED (3 of 14):** source_ip (deprecated; PARSE-REJECT), metadata (deprecated; PARSE-REJECT), custom (extension; PARSE-REJECT — NEW per phase 16 SPEC §1.1 amendment 7).

**Per-route `RBACPerRoute`:** wrapper proto with reserved field 1 + single optional `rbac` field at field 2. Absent (or `rbac: nil`) = disabled-on-route per proto comment + phase 16 SPEC §5.1 (a). Present = wholesale-override per phase 16 SPEC §5.1 (b). Per ADR-0125 §(xii) amendment: phase-16 introduces the **7th canonical per-route pattern** (absent-implies-disabled-OR-wholesale-override; structurally distinct from 5th canonical's explicit-disabled-bool-in-oneof AND 6th canonical's bare-message-via-TPFC). Per-route stats INDEPENDENT per ADR-0145 (mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT precedent).

#### Wire shape

Deny-path wire shape (DENY engine result OR matcher-engine no-match):
- Status: 403 Forbidden.
- Body: byte-exact `"RBAC: access denied"` (19 bytes ASCII; no trailing newline; per phase 16 SPEC §1.1 amendment 10 + §11.P5).
- 4-header set (lowercase wire-form): `content-length: 19`, `content-type: text/plain`, `date: <RFC1123>`, `server: envoy`.
- Connection: keep-alive (no `connection: close`).
- `response_code_details`: envoy-go MVP DEFERS (no emission per phase 16 SPEC §1.1 amendment 11 + §8.12); reference Envoy emits `"rbac_access_denied_matched_policy[<sanitized_policy_id>]"`.

Allow-path wire shape: passthrough — `cb.SendLocalReply` NOT invoked; request forwards to next filter.

#### Per-route INDEPENDENT-stats discipline (per ADR-0145 + phase 16 SPEC §5)

Phase 16 is the THIRD row using stateful-override-with-INDEPENDENT-stats per ADR-0117 precedent (phase-11 local_ratelimit FIRST; phase-15 bandwidth_limit SECOND; phase-16 rbac THIRD). Per-route TPFC entries via `RBACPerRoute{rbac: <RBAC>}` (per ADR-0125 §(xii) NEW 7th canonical) own fresh `*compiledConfig` + fresh `*filterStats` keyed by per-route `rules_stat_prefix`. Listener-level counters do NOT increment for per-route-active streams.

#### Stat surface + Prometheus rendering (per phase 16 SPEC §1.1 amendments 8 + 9)

4 base counters per active namespace combination: `allowed`, `denied`, `shadow_allowed`, `shadow_denied`. Internal stat path (SN2-reuse confirmed at Task 8 empirical scrape per phase 16 SPEC §1.1 amendment 9 + ADR-0145): `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for primary; `http.<HCM_stat_prefix>.rbac.<shadow_rules_stat_prefix>.<counter>` for shadow. Prometheus rendering via existing SN2 default-branch flatten; NO new SN10 rule.

Per-policy counters (when `track_per_rule_stats: true`): `<base_prefix>.policy.<policy_name>.<suffix>` where suffix ∈ {allowed, denied, shadow_allowed, shadow_denied} (the `.policy.` infix segment per ADR-0145 Task 8 empirical scrape refinement to phase 16 SPEC §13.2 hypothesis). Operator-config-driven surface growth; foot-gun documented at `### Phase 16 forward-pointer notes` below.

### envoy.filters.http.jwt_authn

Phase 17 ships `envoy.filters.http.jwt_authn` (Envoy v1.37.2 canonical JWT bearer-token validation filter; decode-side pre-body request gate) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the TENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16). It is the SECOND CONSECUTIVE §9 family-row (after phase-16 rbac) to introduce TWO new framework primitives in a single phase: (i) `internal/jwks/` HTTP-outbound JWKS fetcher per ADR-0150 (see `## JWKS framework primitive`); (ii) `internal/jwt/` JWS/JWT parser + verifier + claim validator per ADR-0151 (see `## JWT verifier framework primitive`). It is the FOURTH §9 family-row to ship pure decode-side (`Encoder: nil`; mirrors phase-12 csrf + phase-13 buffer + phase-16 rbac). It is the FIRST §9 family-row to use the 8th canonical per-route pattern (string-reference-delegation per ADR-0125 §(xiii)) and the FIRST to land an RFC 6750-conformant Bearer-token challenge deny-path. The 8 anchored ADRs are ADR-0148 (package shape + filterStats + boot-registration) + ADR-0149 (compiledConfig shape + consumed-field map) + ADR-0150 (JWKS framework primitive) + ADR-0151 (JWT verifier framework primitive) + ADR-0152 (token extraction) + ADR-0153 (8th canonical per-route) + ADR-0154 (stat surface) + ADR-0155 (deny-path wire shape); ADR-0125 §(xiii) amendment landed at the SPEC commit.

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication` (6 top-level fields per [#next-free-field: 7]):**

| Field | Type | Phase 17 disposition | Notes |
|---|---|---|---|
| `providers` | map<string, JwtProvider> | CONSUMED | Named JWT providers; each carries issuer + audiences + a JWKS source (RemoteJwks XOR LocalJwks) + extraction + side-effect config. |
| `rules` | repeated RequirementRule | CONSUMED | Listener-level dispatch list; first-matching `match` (RouteMatch) wins; resolves to a `requires` requirement OR a `requirement_name` reference. |
| `bypass_cors_preflight` | bool | CONSUMED | When true, OPTIONS preflight requests (`:method == OPTIONS` AND `Origin` present AND `Access-Control-Request-Method` present, per §11.P1 verbatim) bypass JWT validation. |
| `requirement_map` | map<string, JwtRequirement> | CONSUMED | Named-requirement registry; referenced by listener-level `RequirementRule.requirement_name` (parse-rejected on dangling name) AND by per-route `PerRouteConfig.requirement_name` (runtime-resolved on dangling name). |
| `filter_state_rules` | FilterStateRule | SILENT-IGNORED | envoy-go MVP has no filter-state primitive; divergence-window per §1.1 amendment 1 + §8 deferral 12. |
| `strip_failure_response` | bool | CONSUMED | When true, the deny-path SendLocalReply emits an empty body AND no WWW-Authenticate header (both stripped) per §11.P3. |

**`JwtProvider` — 13 of 21 fields consumed (per §1 item 3 + §1.1 amendments 2 + 3 + 7):**

CONSUMED (13): `issuer` + `audiences` + the `remote_jwks` / `local_jwks` oneof + `forward` + `from_headers` + `from_params` + `from_cookies` + `forward_payload_header` + `pad_forward_payload_header` + `claim_to_headers` + `clear_route_cache` + `clock_skew_seconds` (default 60s per §1.1 amendment 7).

SILENT-IGNORED (8): `payload_in_metadata` + `header_in_metadata` + `failed_status_in_metadata` + `normalize_payload_in_metadata` (the dynamic-metadata family) + `jwt_cache_config` (the validated-JWT LRU cache) + `subjects` + `require_expiration` + `max_lifetime` (the v1.37.x claim-coverage extensions). Each is a documented divergence-window — see `### Phase 17 forward-pointer notes`.

**`JwtRequirement` — 6-variant evaluator (per §11.P16):** `provider_name` (validate against the named provider) + `provider_and_audiences` (named provider with a per-rule audience override) + `requires_any` (OR-semantic; short-circuit on first ALLOWED; on all-fail returns the most-informative failure error) + `requires_all` (AND-semantic; short-circuit on first failure) + `allow_missing` (token absent → OK; token present-and-invalid → fail; requires_any-style first-success-wins iteration across providers) + `allow_missing_or_failed` (always ALLOWED). A nil `requires` defaults to `allow_missing_or_failed` per the proto comment.

**Algorithm allow-list:** RS256/384/512 + ES256/384/512 (6 algorithms). HS family + EdDSA + `none` + PS family are DEFERRED — `internal/jwt`'s `VerifySignature` returns `ErrJwtHeaderNotImplementedAlg`.

**Token extraction — 4 sources (per §6.7 + §11.P14 + §11.P15):** iteration order matches Envoy `extractor.cc` — (1) configured `from_headers` in declared order with `value_prefix` substring-search; (2) the default Authorization Bearer header + the default `access_token` query param (active ONLY when no explicit `from_headers` / `from_params` / `from_cookies` is configured for the provider); (3) configured `from_params` (case-sensitive name match; URL-decoded; first-value-only on multi-value); (4) configured `from_cookies` (case-sensitive name match; cookie value verbatim, NO URL-decode). First-success-wins within the provider's source set; empty extraction → `ErrJwtMissed`.

**Side-effect emit-order on a successful validation (per §6.9 + §11.P10 + §11.P13):** FIXED 4-step sequence — (1) strip-on-success: when `forward = false`, strip the extracted token's source (the entire `Authorization` header for the default Bearer source; configured `from_headers` values; `from_params` query-param values via `:path` rewrite; `from_cookies` are UNTOUCHED per the proto caveat); (2) `forward_payload_header`: when non-empty, set the named header to the base64url-encoded JWT payload (padding per `pad_forward_payload_header`); (3) `claim_to_headers`: for each entry, extract the dot-notation claim and set the named header (silent-skip on a missing claim OR an array-valued claim); (4) `clear_route_cache`: when true, invoke `cb.ClearRouteCache()`.

#### Wire shape

Deny-path wire shape (per §4 + §6.9 + §1.1 amendments 8 + 11 + 12 + §11.P1 + §11.P2):
- Status: 401 Unauthorized by default; 403 Forbidden specifically for `JwtAudienceNotAllowed` (mirrors Envoy `filter.cc` `code = (status == Status::JwtAudienceNotAllowed) ? Forbidden : Unauthorized`).
- Body: byte-exact canonical jwt_verify_lib `getStatusString(status)` string — `"Jwt is missing"` (14B) / `"Jwt is expired"` (14B) / `"Jwt verification fails"` (22B) / `"Jwt issuer is not configured"` (28B) / `"Audiences in Jwt are not allowed"` (32B) / `"Jwt not yet valid"` (17B) / etc.
- WWW-Authenticate header: `Bearer realm="<full request URL>"` (the realm is `<scheme>://<authority><path>` — the request's full URL captured at DecodeHeaders entry before any route mutation) + a conditional `, error="invalid_token"` append for every failure-reason EXCEPT `JwtMissed` (the missing-token case omits the error param per §1.1 amendment 12 + RFC 6750 §3).
- `strip_failure_response: true` strips BOTH the body AND the WWW-Authenticate header (empty body, no challenge header) per §11.P3.
- `response_code_details`: envoy-go MVP DEFERS (no emission); reference Envoy emits `jwt_authn_access_denied{<failure_reason_with_spaces_as_underscores>}` per §1.1 amendment 11 + §8 deferral 13. Divergence-window; joint pickup with phase-16 rbac at a future response-code-details framework phase.
- Per-route runtime-resolve error (dangling `requirement_name`): 403 Forbidden + plain body `"Failed JWT authentication: Wrong requirement_name: <name>"` + NO WWW-Authenticate header (mirrors Envoy `filter_config.cc findPerRouteVerifier`).

Allow-path wire shape: passthrough — `cb.SendLocalReply` NOT invoked; the request forwards to the next filter (with the post-validation side-effects applied to the headers).

CORS-preflight-bypass wire shape: when `bypass_cors_preflight: true` and the request is a CORS preflight, the filter passes the request through (`HeaderContinue`) and increments `cors_preflight_bypassed` — no JWT validation runs.

#### Per-route SHARED-stats discipline (per ADR-0153 + ADR-0154 + phase 17 SPEC §5)

Phase 17's per-route TPFC is the `PerRouteConfig` wrapper proto with a REQUIRED oneof `requirement_specifier` carrying two arms — `disabled` (bool; varint) and `requirement_name` (string; PGV `min_len=1`). The 8th canonical per-route pattern per ADR-0125 §(xiii). Three disposition cases: (a) `disabled: true` → the filter is wholly inactive on the route (no JWT validation, NO counter increments per §1.1 amendment 5); (b) `disabled: false` → the oneof is set to the disabled arm with a false value; the filter falls through to listener-level rules dispatch as if no per-route override existed; (c) `requirement_name: "<name>"` → at request time the listener-level `requirement_map[<name>]` is consulted (runtime-resolve; dangling name → 403 + error string). Per-route stats are SHARED with listener-level (NO INDEPENDENT-stats; the per-route is pure string-reference-delegation and spawns no new policy-evaluation state — mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats; DIVERGES from phase-11 + phase-15 + phase-16 INDEPENDENT-stats). ADR-0153 + ADR-0154 codify.

#### Stat surface + Prometheus rendering (per phase 17 SPEC §1.1 amendments 9 + 10 + §11.P6 + §11.P7)

7 base counters, NO per-provider scaling, NO gauges, NO histograms: `allowed` + `denied` + `cors_preflight_bypassed` + `jwks_fetch_success` + `jwks_fetch_failed` + `jwt_cache_hit` + `jwt_cache_miss`. 5 actively emit under MVP; the last 2 (`jwt_cache_*`) are STRUCTURALLY UNREACHABLE (`jwt_cache_config` silent-ignored per §8 deferral 8 — registered but never incremented). Internal stat path: `http.<HCM_stat_prefix>.jwt_authn.<counter>` — HCM-rooted SN2-reuse (RATIFIED at the Task-13 fixture-0019 empirical scrape; both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`). NO new SN flattening rule; NO new tag-extractor. All 7 counters are registered UNCONDITIONALLY at `New()` time. `cors_preflight_bypassed` is the canonical Envoy name per §1.1 amendment 10 (NOT the BRAINSTORM-hypothesized `bypassed_cors_preflight`). Note: reference Envoy increments `allowed` on the CORS-bypass + per-route-disabled passthrough paths; envoy-go MVP does not — see the `allowed` divergence-window at `### Phase 17 forward-pointer notes`.

### envoy.filters.http.ext_authz

Phase 18.1 ships `envoy.filters.http.ext_authz` in **HTTP service mode** (Envoy v1.37.2 canonical external-authorization filter; decode-side gate delegating the allow/deny decision to an external HTTP service) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the ELEVENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17). It is the FIFTH §9 family-row to ship pure decode-side (`Encoder: nil`; mirrors phase-12 csrf + phase-13 buffer + phase-16 rbac + phase-17 jwt_authn). It is the FIRST §9 family-row to REUSE an existing ADR-0125 canonical rather than extend the roster (5th-canonical-REUSE; NO ADR-0125 amendment paragraph — see `### Phase 18.1 forward-pointer notes`). **Phase 18.2 extends this subsection to cover gRPC service mode** — the `grpc_service` arm activates (no longer PARSE-REJECTs) via the NEW `internal/grpcclient/` framework primitive (ADR-0158); ADR-0157 §Decision is amended in-place; ADR-0160/0161 grow gRPC-mode portions. The 7 anchored ADRs from 18.1 are ADR-0156 (package shape + filterStats + DECODER-only) + ADR-0157 (dual-mode compiledConfig + grpc_service PARSE-REJECT; AMENDED at 18.2 — grpc_service now activates) + ADR-0159 (HTTP-outbound auth-check thin local client; disposition (b)) + ADR-0160 (HTTP-mode AuthorizationRequest builder + request-side header filtering; gRPC-mode portion at 18.2) + ADR-0161 (HTTP-mode bidirectional header-mutation discipline; gRPC-mode portion at 18.2) + ADR-0162 (with_request_body ADR-0128 reuse + over-limit 413) + ADR-0163 (per-route 5th-canonical REUSE + SHARED-stats + 6-counter stat surface; NO ADR-0125 amendment). Phase 18.2 adds ADR-0158 (gRPC-client framework primitive — full §Decision + §Consequences) + ADR-0165 (callback-surface extension — 6 new `DecoderFilterCallbacks` methods; ADR-0044 escape-valve fired per planner-time D3 + D12) + ADR-0166 (plaintext h2c upstream relaxation — cluster-manager amendment landing in the same phase).

**INSERTION NOTE (landing-chronological fallback per planner-time decision D10):** SPEC planned alphabetical-after-csrf insertion; BEHAVIOR_CONTRACT.md `## HTTP filter chain` subsections are ordered landing-chronologically (fault@09 → … → jwt_authn@17), so ext_authz lands AFTER jwt_authn per the established fallback.

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.ext_authz.v3.ExtAuthz` — consumed vs deferred:**

| Proto field | envoy-go 18.1 disposition |
|---|---|
| `services` oneof | PGV-NOT-REQUIRED — factory PARSE-REJECTs empty `services`; `http_service` arm builds the HTTP-mode `checkFn` via `buildHTTPCheckFn` (phase 18.1); `grpc_service` arm builds the gRPC-mode `checkFn` via `buildGRPCCheckFn` (phase 18.2 — ADR-0157 §Decision AMENDMENT). Within `grpc_service`: `envoy_grpc.cluster_name` CONSUMED (PGV `min_len: 1`; PARSE-REJECT on cluster-not-found OR `cluster.UseH2() == false`); `google_grpc` arm PARSE-REJECTs envoy-go-strict (`"ext_authz: grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)"`); `timeout` CONSUMED via `context.WithTimeout`; `initial_metadata` + `retry_policy` SILENT-IGNORED per 18.2 SPEC §2.6 + §8 items 2+3. |
| `http_service.server_uri` | CONSUMED. The outbound POST target. |
| `http_service.path_prefix` | CONSUMED. Prepended to the request path. |
| `http_service.authorization_request` | CONSUMED. `headers_to_add` static headers appended to the outbound request; deprecated `allowed_headers` honored-if-present per ADR-0160. |
| `http_service.authorization_response` | CONSUMED. `allowed_upstream_headers` + `allowed_upstream_headers_to_append` + `allowed_client_headers` per ADR-0161. |
| `transport_api_version` | CONSUMED (validation only). Non-V3 PARSE-REJECTs. |
| `with_request_body` | CONSUMED. `BufferSettings{max_request_bytes, allow_partial_message, pack_as_bytes}` — ADR-0128 decode-side body-buffering reuse; `allow_partial_message:false` over-limit → local 413 + `connection: close` per ADR-0162. |
| `failure_mode_allow` | CONSUMED. When true, errors pass the request through (`HeaderContinue`). |
| `failure_mode_allow_header_add` | CONSUMED. When true AND `failure_mode_allow:true` AND an error was bypassed: adds `x-envoy-auth-failure-mode-allowed: true` to the upstream request. |
| `status_on_error` | CONSUMED. `HttpStatus.code` emitted on `failure_mode_allow:false` error path; default `403` if unset. |
| `clear_route_cache` | CONSUMED. Calls `cb.ClearRouteCache()` on the allow path. |
| `validate_mutations` | CONSUMED. Gates header-name/value safety validation (pseudo-header + invalid-token-char reject) on the header-mutation paths (allow + deny). Rejected mutations increment the `invalid` counter. |
| `allowed_headers` | CONSUMED. Top-level `ListStringMatcher` — request-side allow-list applied to outbound auth request headers (both modes). |
| `disallowed_headers` | CONSUMED. Top-level `ListStringMatcher` — removes headers from the outbound auth request even if `allowed_headers` would have allowed them. |
| `stat_prefix` | CONSUMED. Extends the stat namespace via `http.<stat_prefix>.ext_authz.*`. |
| `filter_enabled` / `filter_enabled_metadata` / `deny_at_disable` | SILENT-IGNORED. Runtime family + matcher/metadata family. `disabled` counter STRUCTURALLY UNREACHABLE under MVP (see below). |
| four `*metadata_context_namespaces` fields | SILENT-IGNORED. Dynamic-metadata family. |
| `enable_dynamic_metadata_ingestion` / `filter_metadata` / `bootstrap_metadata_labels_key` | SILENT-IGNORED. Dynamic-metadata / node-metadata family. |
| `charge_cluster_response_stats` | SILENT-IGNORED. Cluster-stat-tree charging family — cluster-scoped stat triple DEFERRED per ADR-0163. |
| `emit_filter_state_stats` | SILENT-IGNORED. Filter-state/access-log family. |
| `decoder_header_mutation_rules` | SILENT-IGNORED. Per-rule mutation-rejection surface (distinct from MVP `validate_mutations`). |

**`HttpService` — all 4 active sub-fields consumed:** `server_uri` + `path_prefix` + `authorization_request` + `authorization_response`.

#### HTTP-outbound auth-check (per ADR-0159; disposition (b))

Phase 18.1 introduces a **thin ext_authz-local HTTP client** in `check.go` (an `httpAuthClient` type wrapping `*http.Client` + configured `HttpService.server_uri.timeout` + `path_prefix`). This is a **new one-way framework primitive** — the per-request HTTP-outbound auth-check POST — composing against the phase-17 `internal/jwks/Fetcher` outbound-HTTP structure (same `http.Client` + timeout discipline) WITHOUT generalizing into a shared `internal/httpclient/` package. SPEC author's disposition (b): two consumers whose lifecycles barely overlap (JWKS fetcher is a cached/async-refreshing long-lived fetcher; ext_authz is a synchronous-per-request cancellable POST-and-parse). The natural trigger to generalize into `internal/httpclient/` is the THIRD outbound-HTTP consumer (anticipated: oauth2 token-endpoint flows — same synchronous-per-request POST shape; see `### Phase 18.1 forward-pointer notes`). ADR-0159 records the disposition + the oauth2-generalization forward-pointer.

The outbound check is **async** — `DecodeHeaders` fires a goroutine that POSTs to the auth service; the decode dispatch goroutine parks via `StopIteration` + a per-stream resume channel (mirrors the phase-09 fault async-resume primitive: `StopIteration` + goroutine + `cb.ContinueDecoding()` on completion). `OnDestroy` cancels the in-flight call's `context.Context` (the FIRST §9 row with a per-request cancellable outbound call). **Cross-phase reuse:** see `## JWKS framework primitive` (phase-17) for the outbound-HTTP structural precedent.

#### Request-side header filtering (per ADR-0160)

`allowed_headers` is a `ListStringMatcher` (top-level, both modes) acting as an allow-list applied to the set of client request headers forwarded to the auth service. `disallowed_headers` overrides and removes any header that `allowed_headers` would have allowed. `AuthorizationRequest.headers_to_add` static headers are appended after allow-list filtering. The deprecated `AuthorizationRequest.allowed_headers` (proto-deprecated; `#1`) is honored-if-present (backward-compat per parent SPEC §6 amendment 2 + ADR-0160) when the top-level `allowed_headers` is absent. `path_prefix` is prepended to the `:path` before forwarding.

**StringMatcher subset honored:** `exact` + `prefix` + `suffix` + `contains` (with `ignore_case`) + `safe_regex` (RFC 2396-syntax subset per the phase-09/12 RegexMatcher discipline) — all COMPILED at parse time. `custom` PARSE-REJECTs per parent SPEC §6 amendment 2 + the revive-ADR-0101 discipline.

#### HTTP-mode bidirectional header-mutation discipline (per ADR-0161)

**Allow path (HTTP 200 from the auth service):**
- `AuthorizationResponse.allowed_upstream_headers` — inject these auth-response headers into the upstream request (set/overwrite).
- `AuthorizationResponse.allowed_upstream_headers_to_append` — append these auth-response headers to the upstream request.
- If `failure_mode_allow_header_add: true` AND the auth-error path was bypassed via `failure_mode_allow:true`: inject `x-envoy-auth-failure-mode-allowed: true` into the upstream request.
- If `clear_route_cache: true`: call `cb.ClearRouteCache()`.

**Deny path (recognized deny status — 401 or 403 from the auth service):**
- Emit `SendLocalReply(status, body, headers)` where:
  - `status` — the auth service's HTTP response status (the FIRST §9 row whose deny-path status is NOT fixed by the filter).
  - `body` — the auth service's HTTP response body, reproduced **verbatim**.
  - `headers` — auth-service response headers filtered through `AuthorizationResponse.allowed_client_headers`; `content-type: text/plain` synthesized as fallback if not present. Decision headers emitted FIRST; framework housekeeping (`content-length`, `date`, `server: envoy`) appended after by the downstream chain framework. NO `x-envoy-*` added on deny.
- `content-length` synthesized by the `SendLocalReply` framework primitive (ADR-0085).

**`validate_mutations` gating:** when `validate_mutations: true`, header names and values on the mutation paths are validated — `:` pseudo-headers REJECTED, invalid token chars in names REJECTED, bare CR/LF/NUL in values REJECTED. Rejected mutations increment the `invalid` counter.

**DEFERRED mutations:** `allowed_client_headers_on_success` (decode-side-only filter shape; no encode leg); `query_parameters_to_set` / `query_parameters_to_remove` (path-query subsystem ADR-0112); `dynamic_metadata_from_headers` (dynamic-metadata family). See `### Phase 18.1 forward-pointer notes`.

#### Request body inclusion (per ADR-0162)

`with_request_body{max_request_bytes, allow_partial_message, pack_as_bytes}` materializes the request body via the phase-13 ADR-0128 decode-side body-buffering reuse (SECOND consumer of ADR-0128 after phase-15 bandwidth_limit; FIRST to consume it for outbound transmission). When `with_request_body` is set and the request has a body, `DecodeHeaders` returns `HeaderStopIteration` and the body accumulates via `DecodeData`. `pack_as_bytes: false` (default) sends the body as a string field; `pack_as_bytes: true` sends as raw bytes. `allow_partial_message: false` (default) + an over-`max_request_bytes` body → `SendLocalReply(413, "Payload Too Large", {connection: close})` BEFORE the outbound auth check fires; NO `ext_authz` counter increments (the request never reached a disposition). `allow_partial_message: true` truncates to `max_request_bytes` and continues.

#### Failure mode + error posture

The **error classification boundary** (per parent SPEC §5.P10): connect failure, timeout, context-canceled → `error` disposition. Recognized deny statuses: `401`, `403`. All other HTTP statuses from the auth service → `error`.

- `failure_mode_allow: false` (proto default) + error → `SendLocalReply(status_on_error, "", {})`. `status_on_error.code` default `403` if unset. `error` counter increments.
- `failure_mode_allow: true` + error → `HeaderContinue` (request passes through). If `failure_mode_allow_header_add: true`: inject `x-envoy-auth-failure-mode-allowed: true` upstream. Both `error` AND `failure_mode_allowed` counters increment.

#### Per-route discipline — 5th-canonical REUSE (NO new canonical) + SHARED-stats (per ADR-0163)

`ExtAuthzPerRoute` carries a **PGV-required** `oneof override` with two arms:
- `disabled` (bool, PGV `const: true`) — `ExtAuthzPerRoute{disabled: true}` wholly deactivates the filter on this route: `DecodeHeaders` returns `HeaderContinue` immediately, NO auth check, NO counter increments. `disabled: false` is PARSE-REJECTED (PGV `const: true` constraint — records as a minor PGV wrinkle vs the bare buffer/compressor 5th canonical's unconstrained disabled-bool, but does NOT constitute a new canonical per ADR-0163).
- `check_settings` (`*CheckSettings`, PGV `required` within the arm) — a NARROWER per-route override carrying: `context_extensions` (`map[string]string` — gRPC-mode-only per its proto doc-note; PARSES but has no HTTP-mode effect, documented as no-op-in-HTTP-mode); `disable_request_body_buffering` (`bool` — overrides listener-level `with_request_body` to OFF on this route); `with_request_body` (`*BufferSettings` — per-route body-buffering override; mutually exclusive with `disable_request_body_buffering`).

This maps onto **ADR-0125's existing 5th canonical** (disabled-bool arm + NARROWER override sub-message arm in a oneof). **Phase 18 lands NO ADR-0125 amendment paragraph** — the FIRST §9 family-row since phase 13 to REUSE an existing canonical rather than extend the roster (breaking the phase-13→17 per-phase-roster-growth streak). ADR-0163 records the explicit no-amendment 5th-canonical-REUSE classification.

**Per-route stats SHARED with listener-level** — the per-route override adjusts `context_extensions`/buffering but still calls the same auth service. No new stateful policy-evaluation state; MIRRORS phase-12 csrf + phase-13 buffer + phase-14 compressor + phase-17 jwt_authn SHARED-stats. Scenario 6 (`ExtAuthzPerRoute.disabled:true`) + scenario 7 (`ExtAuthzPerRoute.check_settings`) exercised in fixture 0020.

**Phase 18.2 gRPC-mode addition (per 18.2 SPEC §5):** the `CheckSettings.context_extensions` `map[string]string` field — silent-ignored in HTTP-mode per the proto's gRPC-mode-only doc-note — is now CONSUMED in gRPC mode. The merged listener+per-route `context_extensions` map (per-route wins on key conflicts per the proto map-merge convention) populates the per-Check `AttributeContext.context_extensions`. Fixture 0021 scenario 7 exercises the merge and asserts the auth-server received-CheckRequest carries the expected `context_extensions` content. Per-route stats remain SHARED with listener-level (the §5 5th-canonical REUSE is mode-agnostic; the per-route `context_extensions` override does NOT introduce per-route stat split).

#### gRPC-mode `AttributeContext` populated set (per ADR-0160 gRPC-mode portion + parent §5.P4 + 18.2 SPEC §11.P4)

In gRPC mode, each per-stream `Check()` call builds an `envoy.service.auth.v3.AttributeContext` proto via `buildAttributeContext` (a pure function of the extended `*authRequest` + 4 config booleans `includePeerCertificate` / `includeTlsSession` / `encodeRawHeaders` / `packAsBytes`). The populated set per parent §5.P4 RATIFIED + §11.P4 in-session refinement:

- **`source.address.socket_address`** — downstream remote IP + port captured at HCM dispatch via `DecoderFilterCallbacks.DownstreamRemoteAddr()` (1 of the 6 new callback accessors landed at Task 4 per ADR-0165).
- **`destination.address.socket_address`** — local listener IP + port via `DecoderFilterCallbacks.DownstreamLocalAddr()`.
- **`source.principal`** — first-value of the phase-16 ADR-0144 `DownstreamPrincipal()` accessor (URI SAN | DNS SAN | Subject DN CN priority order). Empty for plaintext / non-mTLS connections.
- **`destination.principal`** — populated AUTOMATICALLY from the listener TLS cert via `DecoderFilterCallbacks.ListenerPrincipal()` per §11.P4 in-session RATIFICATION — NOT gated by `include_peer_certificate`. The listener-cert URI SAN / DNS SAN / CN is captured at HCM dispatch alongside the ADR-0144 plumbing pattern (no new TLS-layer lift per 18.2 SPEC §11.P13).
- **`request.http.{id, method, headers, path, host, scheme, size, protocol, body | raw_body}`** — pseudo-headers (`:authority` / `:method` / `:path` / `:scheme`) INCLUDED in the headers map, lowercased keys. The HCM-injected headers `x-envoy-auth-partial-body` (set when `with_request_body` materializes a partial body) + `x-forwarded-proto` (= request scheme) + `x-request-id` (UUID) are visible in the headers map per §11.P4 in-session finding (these are HCM-injected before ext_authz runs, not ext_authz-specific). `body` (string) populated when `pack_as_bytes: false` (default); `raw_body` (bytes) when `pack_as_bytes: true`. `protocol` from `DecoderFilterCallbacks.DownstreamProtocol()` (e.g., `HTTP/1.1`, `HTTP/2`).
- **`request.time`** — `*timestamppb.Timestamp` constructed via `timestamppb.Now()` (or `timestamppb.New(streamStartTime)` if plumbed) at builder entry.
- **`tls_session.sni`** — populated ONLY when `include_tls_session: true` AND the downstream connection is TLS, captured via `DecoderFilterCallbacks.DownstreamTLSServerName()`. Per §11.P4 in-session RATIFICATION ONLY `sni` populates from the proto's `TLSSession` message (other fields like `subjectAltName` are NOT populated).
- **`source.certificate`** — DER-encoded leaf cert populated ONLY when `include_peer_certificate: true` AND the downstream presented a client cert, via `DecoderFilterCallbacks.DownstreamTLSPeerCertDER()`.
- **`context_extensions`** — merged listener-level + per-route `CheckSettings.context_extensions` (per-route wins on conflicts). The listener-level contribution is empty for MVP — `initial_metadata` is silent-ignored per §8 item 2.
- **`metadata_context` + `route_metadata_context`** — populated as empty messages (deferred dynamic-metadata family per 18.2 SPEC §8 item 2 — joint with 18.1 + 17 + 16's same-family deferrals).
- **`encode_raw_headers` toggle (per 18.2 SPEC §6.6 step 7 + §12 item 6):** CONDITIONALLY DEFERRED for the `header_map` arm — MVP populates the legacy `headers` map regardless of the flag value; both Envoy and envoy-go produce IDENTICAL `headers` maps when `encode_raw_headers: false` (the default). The `header_map` field population path is DEFERRED per D6 — a future enhancement when an operator-visible behavior gap surfaces.

**CONTRAST with HTTP-mode:** HTTP mode constructs an outbound HTTP POST to the auth service with a request-side `allowed_headers`-filtered header set (per ADR-0160 HTTP-mode portion). gRPC mode constructs a structured proto `AttributeContext` carrying the populated set above and dispatches via `*grpcclient.AuthClient.Check(ctx, *CheckRequest)`. There is NO request-side `allowed_headers` filtering in gRPC mode — the headers map in `AttributeContext.request.http.headers` is the unfiltered HCM-visible header set. The proto's `disallowed_headers` field still applies symmetrically (it removes headers from the outbound CheckRequest's `request.http.headers` map).

#### gRPC-mode `OkHttpResponse` mutation + `DeniedHttpResponse` verbatim pass-through (per ADR-0161 gRPC-mode portion)

The `CheckResponse` → `checkDisposition` mapping (`mapGRPCResponse` in `check.go`) dispatches on `resp.Status.Code`:

- **Allow path** (`Status.Code == 0` AND `HttpResponse` is `*CheckResponse_OkResponse` OR oneof nil):
  - `OkHttpResponse.headers` (`[]*core.HeaderValueOption`) — 4-arm `append_action` dispatch per phase-10 header_mutation precedent (ADR-0161): `APPEND_IF_EXISTS_OR_ADD` → upstream append; `OVERWRITE_IF_EXISTS_OR_ADD` / `OVERWRITE_IF_EXISTS` → upstream set; `ADD_IF_ABSENT` → upstream conditional add.
  - `OkHttpResponse.headers_to_remove` — populated into `upstreamDel` (the framework's upstream-delete primitive).
  - `OkHttpResponse.response_headers_to_add` — **SILENT-IGNORED per D11 + 18.2 SPEC §8 item 5** (decode-side-only filter shape; no encode leg; same family as HTTP-mode's `allowed_client_headers_on_success`). Operator-divergence-window documented in §13.4 Phase 18.2 forward-pointer notes.
  - `OkHttpResponse.query_parameters_to_set` + `query_parameters_to_remove` — DEFERRED (path-query rewriting subsystem ADR-0112).
  - `OkHttpResponse.dynamic_metadata` + top-level `CheckResponse.dynamic_metadata` — DEFERRED (dynamic-metadata family).

- **Deny path** (`Status.Code != 0` AND `HttpResponse` is `*CheckResponse_DeniedResponse` OR oneof nil):
  - `DeniedHttpResponse.status.code` — auth-decision status code (e.g., 401, 403); empty/zero falls back to default 403 per parent §5.P11.
  - `DeniedHttpResponse.body` — auth-service response body reproduced **verbatim** (parent §5.P11).
  - `DeniedHttpResponse.headers` — applied **VERBATIM** to the deny-path `SendLocalReply` — **UNLIKE HTTP-mode** which filters auth-response headers through `AuthorizationResponse.allowed_client_headers`. This is the central wire-shape contrast between the two modes: gRPC mode trusts the auth service's header decision entirely; HTTP mode applies a parse-time-compiled matcher set as a safety boundary. Operator-divergence-window: a misbehaving gRPC auth service can inject arbitrary headers into the downstream response; HTTP-mode operators have the `allowed_client_headers` knob as an additional safety lever. Fixture 0021 scenario 2 (deny path) asserts the verbatim header pass-through.

- **Empty CheckResponse** (oneof nil, `Status.Code == 0`, no `HttpResponse`) → `dispAllow` (parent §5.P11).
- **Envoy-go-strict edge cases** (per 18.2 SPEC §6.7 + parent §5.P10):
  - `OkResponse` + non-zero `Status.Code` → `dispError` (well-formed responses must not mix `OkResponse` with denial-status).
  - `DeniedResponse` + zero `Status.Code` → `dispError` (well-formed responses must not mix `DeniedResponse` with success-status).
  - Transport-level errors (gRPC connect failure, timeout, `ctx.Err()`) → `dispError` per parent §5.P10; `failure_mode_allow` / `status_on_error` posture applies identically to both modes.
- **`validate_mutations` gating** applies identically to both modes — a header-name/value safety violation in `upstreamSet` / `upstreamApp` / `denyHeaders` → `dispInvalid` → `invalid` counter increment + error posture per ADR-0156.

#### gRPC dial + connection lifecycle (per ADR-0158)

The `internal/grpcclient/` package exposes a generic `Dialer` (cluster-name → `*grpc.ClientConn` via `grpc.WithContextDialer((*cluster.Cluster).Dial)` + `WithTransportCredentials(insecure.NewCredentials())` — TLS terminates at the cluster-manager layer per 18.2 SPEC §11.P13 in-session RATIFICATION; gRPC's own TLS layer is bypassed) + a thin ext_authz-typed `AuthClient` wrapper (`Check(ctx, *CheckRequest) (*CheckResponse, error)`; `envoy.service.auth.v3.Authorization/Check` stub from go-control-plane v1.32.4 — no codegen). One `*grpc.ClientConn` per `(cluster_name, compiledConfig)` pair created at config-load time in `buildGRPCCheckFn` and shared across all per-stream `Check()` calls (gRPC manages its own transport-level reconnect). Closed-on-process-exit for MVP — no `Close()` call (per §8 item 2 deferred). **ADR-0166 (plaintext h2c upstream relaxation)** is the same-phase cluster-manager amendment that permits non-TLS h2c upstream clusters via `cluster.Dial` returning a plaintext `net.Conn` (relaxing the prior TLS-required gate in `extractH2Mode` / `dial_h2.go`); this is required to make fixture 0021's three-listener topology load the plaintext h2c auth-cluster. See `## gRPC client framework primitive (per phase 18.2 ADR-0158)` below for the full umbrella.

#### Wire shape

**Allow path:** passthrough — `cb.SendLocalReply` NOT invoked; the request forwards to the next filter (with the post-validation allow-path header mutations applied).

**Deny path wire shape** (per SPEC §4 + parent §5.P11 empirical; see §18.P11 RATIFIED at Task 13):
- Status: the auth service's HTTP response status (401 or 403).
- Body: auth service's HTTP response body, reproduced verbatim.
- Headers (in order): decision headers filtered through `allowed_client_headers` FIRST; framework housekeeping (`content-length`, `date`, `server: envoy`) AFTER.
- `content-type: text/plain` synthesized as fallback if the auth service did not supply one in the allowed set.
- NO `x-envoy-*` header added on deny.

**Error path:** `SendLocalReply(status_on_error, "", {})` when `failure_mode_allow: false`; passthrough + `x-envoy-auth-failure-mode-allowed` when `failure_mode_allow: true`.

**gRPC-mode wire shape (per 18.2 SPEC §6.7 + fixture 0021 scenarios):** allow path is passthrough mode-agnostic (with `OkHttpResponse.headers` upstream mutation + `headers_to_remove` applied). Deny path is `SendLocalReply(status, body, headers)` where `status` / `body` / `headers` are drawn from `DeniedHttpResponse` (VERBATIM headers — UNLIKE HTTP-mode's `allowed_client_headers` filtering). Error path is mode-agnostic (`status_on_error` posture for `failure_mode_allow: false`; passthrough + `x-envoy-auth-failure-mode-allowed` for `failure_mode_allow: true`).

**`response_code_details` NOT emitted** — envoy-go MVP defers; reference Envoy emits `ext_authz_denied`. Joint divergence-window with phase-16 rbac + phase-17 jwt_authn; see `### Phase 18.1 forward-pointer notes` + `### Phase 18.2 forward-pointer notes`.

#### Stat surface + Prometheus rendering (per ADR-0163 + parent SPEC §6 amendment 8 + §18.P6 RATIFIED at Task 8)

6 base counters, NO gauges, NO histograms: `ok` + `denied` + `error` + `disabled` + `failure_mode_allowed` + `invalid`. 5 actively emit under MVP; `disabled` is STRUCTURALLY UNREACHABLE under MVP — registered unconditionally at `New()` time for scrape-stability, never incremented (publishes 0; couples to the deferred runtime `filter_enabled` gate). Internal stat path: `http.<HCM_stat_prefix>.ext_authz.<counter>` — HCM-rooted SN2-reuse (RATIFIED at the Task-8 empirical scrape; both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`). NO new SN flattening rule; NO new tag-extractor. All 6 counters registered UNCONDITIONALLY at `New()` time. Per-route stats SHARED with listener-level (ADR-0163 SHARED-stats — no per-route `*filterStats`).

**Phase 18.2 stat-surface delta — ZERO new counters (per 18.2 SPEC §13.2):** the 6 ext_authz counters are MODE-AGNOSTIC. The gRPC-mode `mapGRPCResponse` dispatch increments the same `ok` / `denied` / `error` / `failure_mode_allowed` / `invalid` counters as HTTP-mode's `mapHTTPResponse` dispatch — the counter family is the filter's per-stream disposition tally, not a transport-mode discriminator. Stat-table total remains at **77 internal names** (UNCHANGED from 18.1).

### envoy.filters.http.ext_proc

Phase 19.1 ships `envoy.filters.http.ext_proc` in **headers-stages-only mode**; **phase 19.2 EXTENDS to body-stage participation in BUFFERED mode for the gRPC-service arm** (Envoy v1.37.2 canonical external-processor filter — both-decode-and-encode-side; delegates per-stage mutation decisions for both the request AND the response to an external service over a bidirectional gRPC stream OR a JSON-transcoded HTTP POST per stage) per the canonical Envoy v1.37.2 filter spec under the 07.1 framework. envoy-go's MVP envelope is the TWELFTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2). It is the FIRST §9 family-row to ship BOTH `StreamDecoderFilter` AND `StreamEncoderFilter` participation in a single filter package (phase-14 compressor's encode-only is the only prior encode-side precedent; the BOTH-DECODE-AND-ENCODE shape is anchored at ADR-0167). **Body-stage activation (gRPC-service arm, BUFFERED only) lands at phase 19.2** (sibling sub-phase). 19.2 LIFTS the 19.1 PARSE-REJECT for `request_body_mode = BUFFERED` + `response_body_mode = BUFFERED` on the gRPC-service arm per ADR-0168 §Decision AMENDMENT; STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED continue PARSE-REJECT envoy-go-strict permanently per parent §4.4; HTTP-service-mode body-mode PARSE-REJECT continues per the proto's `ExtProcHttpService` constraint. Trailer-modes `!= SKIP` PARSE-REJECT permanently. Three STREAMED-only top-level flags (`observability_mode`, `send_body_without_waiting_for_header_response`, `deferred_close_timeout != 0`) PARSE-REJECT permanently per parent §5.P10. **8 anchored ADRs + 4 §Decision-touchpoints at 19.2** (ADR-0167..ADR-0174 from 19.1 carry forward; **ADR-0175 §Decision + §Consequences** for the NEW `EncoderFilterCallbacks.BufferEncodedBody` framework primitive + **ADR-0168 / ADR-0171 / ADR-0172 §Decision AMENDMENTS** in-place per ADR-0044): ADR-0167 (package shape + BOTH-decode-encode filter + 9-counter filterStats + multi-stage `SendLocalReply`); ADR-0168 (compiledConfig + grpc_service-vs-http_service mutex + body-mode 19.1 PARSE-REJECT); ADR-0169 (`*ProcessorClient` bidi-stream wrapper EXTENDING the phase-18.2 `*Dialer` — FIRST cross-phase consumer of ADR-0158); ADR-0170 (filter-local `ProcessingRequest`/`ProcessingResponse` JSON codec); ADR-0171 (header-mode ProcessingMode state machine + mode-override + max_message_timeout); ADR-0172 (header_mutation + ImmediateResponse-at-headers + clear_route_cache/route_cache_action + grpc-status content-type sniff); ADR-0173 (per-route 5th-canonical REUSE + SHARED-stats + 9-counter stat surface; NO ADR-0125 amendment — SECOND CONSECUTIVE §9 REUSE after phase 18.1); ADR-0174 (symmetric `EncoderFilterCallbacks` extension — 6 new methods mirroring ADR-0165; reuses chain-field plumbing). **D12 hypothesis HELD** at 19.1 IMPL phase-done — NO impl-time-unanticipated ADR fired; ADR-0177 stays unconsumed (reserved for 19.2).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.ext_proc.v3.ExternalProcessor` — consumed vs deferred:**

| Proto field | envoy-go 19.1 disposition |
|---|---|
| `grpc_service` (#1) / `http_service` (#20) | Mutually exclusive per parent §5.P1; both-set OR neither-set PARSE-REJECT. `grpc_service` arm builds the bidi-stream client via ADR-0169 `*grpcclient.ProcessorClient` (within `grpc_service`: `envoy_grpc.cluster_name` CONSUMED — PARSE-REJECT on cluster-not-found OR `cluster.UseH2() == false`; `google_grpc` PARSE-REJECTs envoy-go-strict per ADR-0157 §Decision AMENDMENT inherited; `timeout` CONSUMED via `context.WithTimeout`; `initial_metadata` + `retry_policy` SILENT-IGNORED). `http_service` arm builds the filter-local `httpProcessorClient` wrapping `*http.Client` with the parsed base URL from `http_uri.uri` + `http_uri.timeout`; the proto-constraint at `ExtProcHttpService` requires body-mode == NONE (PARSE-REJECT at the listener-level body-mode gate). |
| `processing_mode` (#3) | CONSUMED per parent §5.P9. `HeaderSendMode.DEFAULT (0)` → SEND for header-modes; SKIP for trailer-modes. **`BodySendMode = BUFFERED` ACCEPT-AND-WIRE for the gRPC-service arm per ADR-0168 §Decision AMENDMENT (phase-19.2 IMPL Task 3 — the body-mode PARSE-REJECT lift).** STREAMED-class arms (STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED) continue PARSE-REJECT permanently. HTTP-service-mode body-mode PARSE-REJECT continues per the proto's `ExtProcHttpService` constraint. `HeaderSendMode != SKIP` on trailer-modes PARSE-REJECT permanently. |
| `request_attributes` (#5) / `response_attributes` (#6) | CONSUMED as `[]string` allowlists per SPEC §6.6 — populates `ProcessingRequest.attributes` envelope at the request_headers / response_headers stages respectively. The CEL attribute-name registry mapping to envoy-go callback accessors lands at `internal/filter/http/extproc/attributes.go` (Task 9). Per §19.P8 the wire-shape RATIFIED on the codec-options axis (snake_case + omit-zero + enum-as-string protojson options matching reference Envoy v1.37.2; three envelope-content divergences deferred to 19.2 per ADR-0170 §Consequences). |
| `failure_mode_allow` (#7) / `message_timeout` (#10) / `max_message_timeout` (#15) / `disable_immediate_response` (#16) | Error-posture surface per parent §5.P11. `failure_mode_allow:true` + processor error → stream continues with `HeaderContinue` + `failure_mode_allowed` counter increment (also increments `streams_failed`). `message_timeout` (proto default 200ms; PGV `[0s, 1h]`) — per-stage message timer per ADR-0171 §Decision (vi); `max_message_timeout` GATES `override_message_timeout` enablement (when 0, override API disabled; when ≥1ms, override is range-checked against `[1ms, max_message_timeout]`). `disable_immediate_response:true` → `ImmediateResponse` ProcessingResponses classified as `spurious_msgs_received` + dropped. **Per-message timer LIFTED TO BEHAVIORAL ENFORCEMENT at 19.2** per ADR-0171 §Decision AMENDMENT (phase-19.2 IMPL Task 4): a single rolling per-direction timer via `context.WithTimeout` cancel-and-rebuild (planner-time D4) bounds each in-flight `Recv` against the active per-message timeout; race-tested at IMPL Task 8 (`TestPerMessageTimer_*` group). The 19.1 STRUCTURAL-ONLY carry-forward CLOSES at 19.2. |
| `stat_prefix` (#8) | CONSUMED. Extends the stat namespace via `http.<stat_prefix>.ext_proc.*`. |
| `mutation_rules` (#9) | CONSUMED per parent §5.P3. Pre-compiled at parse time into `*resolvedMutationRules` (four boolean wrappers carrying the protected-default-set semantics — `disallow_is_error` / `disallow_all` / `allow_all_routing` / `allow_envoy`); per-header gating fires at `CommonResponse.header_mutation` application time. Rejected mutations are dropped silently AND set a per-stage flag so `spurious_msgs_received` increments ONCE per stage with any rejection. |
| `forward_rules` (#12) | Pre-compiled at parse time into `*resolvedForwardRules` placeholder; the per-header forwarding-filter application is a 19.2+ surface (the resolution captures the proto pointer; no body-stage forwarding gate fires in 19.1's headers-only envelope). |
| `allow_mode_override` (#14) / `allowed_override_modes` (#22) | CONSUMED per parent §5.P1. `allow_mode_override:false` (default) silently drops all `mode_override` arrivals; when true, each `mode_override` ProcessingResponse on a `request_headers` / `response_headers` response is validated against `allowed_override_modes` (when non-empty, exact-match required — non-matching → `spurious_msgs_received`). Each `allowed_override_modes` entry passes the same `resolveProcessingMode` validation as the listener-level `processing_mode`. |
| `route_cache_action` (#18) / `disable_clear_route_cache` (#11) | Mutually exclusive per parent §5.P5; both-set PARSE-REJECT. `disable_clear_route_cache:true` translates to RETAIN; otherwise the raw enum value (DEFAULT / RETAIN / CLEAR) flows through. CONSUMED at request-headers stage only per the proto-doc's "request_headers stage only" constraint; the response_headers stage emits a 500 LocalReply if the processor attempts a `clear_route_cache:true` (matches reference Envoy v1.37.2 INVALID classification per §19.P7). |
| `observability_mode` (#17) | PARSE-REJECT permanently — STREAMED-only flag; out of envelope per parent §5.P10. |
| `send_body_without_waiting_for_header_response` (#21) | PARSE-REJECT permanently — STREAMED-only flag. |
| `deferred_close_timeout` (#19) | PARSE-REJECT permanently for non-zero — observability_mode-coupled. |
| `metadata_options` / `filter_metadata` | SILENT-IGNORED — dynamic-metadata family (joint with phases 16+17+18 deferrals). |
| `ProcessingResponse.dynamic_metadata` emit + `ProcessingRequest.metadata_context` | SILENT-IGNORED — dynamic-metadata family. |
| `CommonResponse.dynamic_metadata` and `CommonResponse.trailers` | DEFERRED — both proto-flagged `[#not-implemented-hide:]` or dynamic-metadata family. |
| `HttpHeaders.attributes` (deprecated; `[#not-implemented-hide:]`) | SILENT-IGNORED. |
| `ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}` | Silent-ignored per ADR-0173 — the three `[#not-implemented-hide:]` fields (`async_mode`, `request_attributes`, `response_attributes`) at per-route ExtProcOverrides are distinct from the MVP-CONSUMED top-level `ExternalProcessor.request_attributes`/`response_attributes` (#5/#6) which populate the listener-level attribute envelope. |

#### Per-stage state machine + mode-override discipline (per ADR-0171)

Per-direction `ProcessingMode` state — `f.activeProcessingMode *resolvedProcessingMode` initialized from `cc.processingMode` (or per-route override at `DecodeHeaders` entry per ADR-0173) and read by both decode-side (request_headers stage gating) and encode-side (response_headers stage gating). The framework's sequential decode→encode dispatch invariant + the bidi-stream single-in-flight-message correlation rule together guarantee at most ONE filter-side dispatch goroutine is live at any time per stream — the bedrock of the D9 NO-per-stream-mutex race discipline (anchored at ADR-0171 §Consequences).

**Mid-stream `mode_override` re-eval — header-response paths ONLY.** When `ProcessingResponse.mode_override` arrives in response to a `request_headers` or `response_headers` ProcessingRequest AND `cc.allowModeOverride == true` AND the override mode satisfies the `cc.allowedOverrideModes` allowlist (when non-empty), `f.activeProcessingMode` is replaced with the resolved override. `mode_override` arriving on a body-stage or trailer-stage response is silently dropped per proto-doc (NOT classified as `spurious_msgs_received`). `mode_override` outside `allowed_override_modes` IS classified as `spurious_msgs_received`.

**`override_message_timeout` discipline.** When `max_message_timeout >= 1ms`, the override is honored ONCE-per-stage in range `[1ms, max_message_timeout]`; out-of-range OR more-than-once-per-stage OR `max_message_timeout < 1ms` → `override_message_timeout_ignored++`. The `ProcessingResponse` carrying `override_message_timeout` has its OTHER fields IGNORED (short-circuited in `applyProcessingResponse`).

**`mutation_rules` per-header gating.** Each set/remove operation in `CommonResponse.header_mutation` is independently validated against the resolved `mutation_rules`; rejected mutations drop silently AND set a per-stage flag so `spurious_msgs_received++` ONCE per stage with any rejection.

#### Body-stage wire shape — `body_mutation` + `CONTINUE_AND_REPLACE` + body-stage `ImmediateResponse` (per ADR-0168 + ADR-0171 + ADR-0172 + ADR-0175 §Decision AMENDMENTs / NEW at 19.2)

Phase 19.2 ACTIVATES the body-mode wire surface for the gRPC-service arm under `BUFFERED` only. The 4-stage state machine (`stageRequestHeaders` / `stageRequestBody` / `stageResponseHeaders` / `stageResponseBody`) per ADR-0171 §Decision AMENDMENT extends the at-most-once-per-stage discipline from 2 → 4 stages; spurious entries (a second `DecodeData`-endStream on the same stream, etc.) increment `spurious_msgs_received`. The body buffers accumulate via ADR-0128 decode-side `BufferedBody` (REUSED unchanged) + the NEW ADR-0175 encode-side `BufferEncodedBody` (the symmetric mirror landed at 19.2 IMPL Task 2; chain-side `c.encodeBuf` accumulation discipline on `DataStopIterationAndBuffer`; release on `ContinueEncoding()` per `internal/filter/http/chain.go`'s `RunEncodeData` extension).

**Body-stage outbound (`ProcessingRequest{request_body|response_body}`)** carries a `HttpBody` envelope with the accumulated body bytes + the body-stage attribute envelope (CEL roster MIRRORS the header-stage roster per ADR-0170 §Consequences extension + adds the body-stage-natural `request.size` / `response.size` numeric attributes populated from `int64(len(body))` per planner-time D5 disposition HOLDS at fixture-0023 scrape).

**`CommonResponse.body_mutation` (`*BodyMutation` oneof)** dispositions per ADR-0172 §Decision AMENDMENT (SPEC §4.2):

| Oneof arm | 19.2 disposition |
|---|---|
| `body []byte` | CONSUMED — replaces the buffered body bytes; `Content-Length` reconciled on the corresponding header set via existing callback header-mutation API per ADR-0128 §Decision (decode) + ADR-0175 §Decision (encode). |
| `clear_body bool` (true) | CONSUMED — replaces buffered body with empty slice; same `Content-Length: 0` reconciliation. `clear_body:false` is a no-op (buffer unchanged). |
| `streamed_response *StreamedBodyResponse` | PARSE-REJECT — STREAMED is out-of-envelope permanently (per parent §4.4); `applyBodyMutation` returns `(actError, errStreamedResponseBodyMutationUnsupported)` + increments `spurious_msgs_received` per ADR-0172 §Decision (iv) malformed-response discipline. |

**`CommonResponse.status = CONTINUE_AND_REPLACE`** dispositions per ADR-0172 §Decision AMENDMENT (SPEC §4.3):

| Stage where emitted | 19.2 disposition |
|---|---|
| Header stage WITH body-mode = NONE | CONSUMED as no-op for body (header_mutation still applies; the 19.1 spurious-dispatch LIFTS — no counter increment); returns `actContinue`. |
| Header stage WITH body-mode = BUFFERED | CONSUMED as combined header+body replacement — `header_mutation` + `body_mutation` both apply; subsequent body-stage outbound dispatch is SKIPPED via `f.skipBodyStageDispatch[direction] = true` per Task 6 IMPL discipline; returns `actContinueButStillWaiting`. |
| Body stage (request_body / response_body) | TREATED AS CONTINUE per the proto's "ignored at body stages" wording — the body is already in flight; replacement would race with the buffered-body release path. No counter increment. The 19.1 spurious-dispatch for `CONTINUE_AND_REPLACE` at header stages with body-mode = NONE LIFTS at 19.2 per the first row above. |

The 19.1 SPEC §12 deferred decision #7 (`CONTINUE_AND_REPLACE` handling) is SETTLED at 19.2 SPEC per this table.

**Body-stage `ImmediateResponse`** extends the 4-stage `SendLocalReply` mechanism unchanged — request_body + response_body stages now emit `SendLocalReply` with the same proto-faithful status / headers / body / (optional) grpc_status as the 19.1-landing header stages. `clear_route_cache` at body-stage `ImmediateResponse` continues IGNORED. **Empirical scope-handling carry-forward** (fixture 0023 Empirical-pin AMENDMENT (II) at PROGRESS Task 9): envoy-go HCM rejects encode-side `SendLocalReply` after the encode chain has started — closure-deferred to a future phase (likely an HCM-side amendment to the late-arrival path during encode chain execution); the substantive body-stage deny contract delivers via the decode-side request_body path.

#### Deny-path wire shape — multi-stage `ImmediateResponse` (per ADR-0167 + ADR-0172; body-stage extension per 19.2 §Decision AMENDMENT)

`ImmediateResponse` can fire at the **request_headers** stage (decode-side `SendLocalReply` from `dcb`) OR the **response_headers** stage (encode-side emission — the FIRST §9 row to emit `SendLocalReply` from the encode side at the response_headers stage; routes through `dcb` per ADR-0085's framework primitive + ADR-0075 encode-chain entry semantics, NOT via the encode-side callback). **At 19.2 the body stages also fire `SendLocalReply` per the 4-stage extension** — the `request_body` stage fires via `dcb.SendLocalReply` (the well-supported decode-side path; race-tested at IMPL Task 8); the `response_body` stage fires via the encode-side path subject to the HCM late-arrival framework gap noted above (carried as a forward-pointer in `### Phase 19.2 forward-pointer notes`).

- `ImmediateResponse.status.code` — HTTP status code. Default unspecified per proto.
- `ImmediateResponse.body` — response body bytes; reproduced verbatim.
- `ImmediateResponse.headers` (`*HeaderMutation`) — mutation applied to the local-reply header set per the same per-header gating discipline as `CommonResponse.header_mutation`.
- `ImmediateResponse.grpc_status` — translation gate: when the downstream `Content-Type` matches the gRPC content-type sniff (`application/grpc*`), the gRPC-status header `grpc-status` is added to the response with the value from `grpc_status.status`. The grpc-status surface is a header-only emission at 19.1 (no trailer); the 19.2 trailer-emission framework path is signposted.
- `ImmediateResponse.details` — `response_code_details` SILENT-IGNORED at MVP (joint deferral with phases 16+17+18 — envoy-go HCM does not surface response_code_details).

When `disable_immediate_response: true` is set on the listener config, ImmediateResponse arrivals are classified as `spurious_msgs_received` and dropped (no LocalReply emitted; stream continues).

#### `clear_route_cache` + `route_cache_action` precedence (per ADR-0172 + parent §5.P5)

`CommonResponse.clear_route_cache` is honored ONLY at the request_headers stage per the proto-doc constraint. Application precedence:

1. **`disable_clear_route_cache: true`** (listener-level) — UNCONDITIONALLY suppresses `clear_route_cache` regardless of `route_cache_action` value; translates internally to `RETAIN`.
2. **`route_cache_action == CLEAR`** (listener-level) — UNCONDITIONALLY clears the route cache regardless of the per-response `clear_route_cache` value.
3. **`route_cache_action == RETAIN`** (listener-level) — UNCONDITIONALLY retains the route cache.
4. **`route_cache_action == DEFAULT`** (listener-level) — honors per-response `CommonResponse.clear_route_cache` (true → `cb.ClearRouteCache()`; false → no-op).

A `clear_route_cache: true` arriving in a `response_headers`-stage `CommonResponse` triggers a 500 LocalReply matching reference Envoy v1.37.2's INVALID classification (per §19.P7 closure — RATIFIED-BY-CONSTRUCTION).

#### Per-route discipline — 5th-canonical REUSE (NO new canonical) + SHARED-stats (per ADR-0173)

`ExtProcPerRoute` carries a **PGV-required** `oneof override` with two arms:

- `disabled` (bool, PGV `const: true`) — `ExtProcPerRoute{disabled: true}` wholly deactivates the filter on this route: `DecodeHeaders` returns `HeaderContinue` immediately, NO bidi-stream opened, NO counter increments. `disabled: false` PARSE-REJECTED (PGV `const: true`).
- `overrides` (`*ExtProcOverrides`) — a NARROWER per-route override carrying MVP-CONSUMED `processing_mode` + `grpc_service` (per-route grpc_service overrides the listener-level transport target; PARSE-REJECT on cross-mode override — http→grpc or grpc→http). The 5 silent-ignored sub-fields (`async_mode`, `request_attributes`, `response_attributes`, `metadata_options`, `grpc_initial_metadata`) are documented above.

This maps onto **ADR-0125's existing 5th canonical** (disabled-bool arm + NARROWER override sub-message arm in a oneof). **Phase 19.1 lands NO ADR-0125 amendment paragraph** — the SECOND CONSECUTIVE §9 row (after phase 18.1 ext_authz) to REUSE the 5th canonical rather than extend the roster; strengthens the ADR-0125 roster-not-monotonic lesson. ADR-0173 records the explicit no-amendment 5th-canonical-REUSE classification.

**Cache-on-first-use per-route resolution** (per §19.P7 RATIFIED-BY-CONSTRUCTION): `(*filter).resolvePerRoute` is called ONCE at `DecodeHeaders` entry + cached on `f.activePerRoute` for the entire filter lifetime; subsequent stages (including encode-side response_headers) consume the cached resolution. The cache-on-first-use discipline is ESTABLISHED-BY-CONSTRUCTION via the single resolution entry point — no `clear_route_cache`-mid-stream scenario can falsify it (the §19.P7 fixture-harness scrape closure was re-scoped to by-construction per ADR-0173 §Consequences).

**Per-route stats SHARED with listener-level** — the per-route override adjusts processing_mode / grpc_service but spawns NO new policy-evaluation state; MIRRORS phase-12 csrf + phase-13 buffer + phase-14 compressor + phase-17 jwt_authn + phase-18.1+18.2 ext_authz SHARED-stats. Per-route grpc_service overrides do allocate a per-route `*grpcclient.ProcessorClient` (a separate `*grpc.ClientConn` per `(per-route cluster, compiledConfig)` pair, materialized lazily at `(*factoryState).resolvePerRouteConfig` time) but do NOT split the stat surface.

#### Stat surface + Prometheus rendering (per ADR-0173 + §19.P4 RATIFIED-WITH-AMENDMENT)

9 base counters, NO gauges, NO histograms — see `## Stat-name mapping` for the full table. Internal stat path: `http.<HCM_stat_prefix>.ext_proc.<counter>` — HCM-rooted SN2-reuse (RATIFIED at the Task 13 fixture-0022 empirical scrape; both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form `envoy_http_ext_proc_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`). NO new SN flattening rule; NO new tag-extractor. All 9 counters registered UNCONDITIONALLY at `New()` time per the phase-17/18 STRUCTURALLY-UNREACHABLE-counter unconditional-registration discipline.

**§19.P4 RATIFIED-WITH-AMENDMENT (per ADR-0173 §Consequences):** the 9-counter MVP roster is the canonical envoy-go subset; reference Envoy v1.37.2 emits an additional 8 counters NOT in the hypothesis (`immediate_responses_sent`, `message_timeouts`, `clear_route_cache_disabled`, `clear_route_cache_ignored`, `clear_route_cache_upstream_ignored`, `rejected_header_mutations`, `server_half_closed`, `http_not_ok_resp_received`) — DEFERRED to phase 19.2+ activation. The fixture-harness assertion gate at Task 13 is relaxed to "counter NAMES present on BOTH sides" (not "delta values match cross-side") to accommodate the partial-roster MVP — per-scenario delta divergences (e.g., `streams_failed{l_test_a}=2` vs ref 1 on scenario 2; `failure_mode_allowed{l_test_b}=2` vs ref 1 on scenario 5) are DOCUMENTED as 19.2 surfaces under this authorization.

#### Wire shape — gRPC mode

**Allow path:** the per-stage `Process` bidi-stream Send + Recv pair runs to completion; `ContinueDecoding()` / `ContinueEncoding()` fires after `applyProcessingResponse` completes; the request forwards to the next filter (with the validated `CommonResponse.header_mutation` applied + the per-stage `mode_override` applied per ADR-0171).

**Deny path** (`ImmediateResponse` at request_headers or response_headers): `SendLocalReply(status, body, headers)` per ADR-0085; the bidi-stream `CloseSend` fires + `streamCancel` aborts any in-flight Recv. The grpc-status content-type sniff fires when the downstream `Content-Type` matches `application/grpc*`.

**Error path:** transport-level errors (gRPC connect failure, timeout, `ctx.Err()`) → `streams_failed++` + the `failure_mode_allow` posture applies (`failure_mode_allow:true` → `HeaderContinue` + `failure_mode_allowed++`; `failure_mode_allow:false` → `SendLocalReply(500, "", {})`).

#### Wire shape — http_service mode

**Per-stage POST:** each stage marshals the `*ProcessingRequest` envelope via the ADR-0170 protojson codec (snake_case + omit-zero + enum-as-string) + issues a POST against `http_service.http_uri.uri` with `Content-Type: application/json`. The per-call timeout is the `http_uri.timeout` set on the `*http.Client`. On non-2xx → `streams_failed++` + error-posture applies; on 2xx → response body unmarshaled via `unmarshalProcessingResponse` (`DiscardUnknown:true` per the §19.P8 closure) → `completeStage` runs `applyProcessingResponse` identical to gRPC mode.

**`response_code_details` NOT emitted** — envoy-go MVP defers; reference Envoy emits `ext_proc_*` family per stage. Joint divergence-window with phases 16+17+18 — see `### Phase 19.1 forward-pointer notes`.

#### Concurrency + lifecycle (per ADR-0171 + ADR-0167)

- **One `*grpc.ClientConn` per `(cluster_name, compiledConfig)` pair** (gRPC mode) — shared across all per-stream `Process` calls; long-lived; closed-on-process-exit for MVP. Per-route `grpc_service` overrides allocate their own `*ProcessorClient` lazily at first per-route resolution time.
- **Bidi-stream half-close + `CloseSend`** — the client calls `CloseSend()` after the final stage's response completes OR on `ImmediateResponse` arrival OR on `OnDestroy` cancellation (`streamCancel()` propagates).
- **`OnDestroy` sync.Once-guarded** — idempotent against multiple OnDestroy invocations + concurrent OnDestroy + dispatch-completion races per the D9 race discipline.

### envoy.filters.http.oauth2

Phase 20 ships `envoy.filters.http.oauth2` (Envoy v1.37.2 canonical OAuth 2.0 authentication filter — a decoder-only filter that delegates sign-in via a 302 challenge to the configured `authorization_endpoint`, completes the auth-code exchange via an async-resumed POST to the `token_endpoint`, encrypts the access + refresh tokens via AES-256-CBC, emits a 5-cookie envelope per request, silently rotates expired bearer tokens via the `refresh_token` grant, clears the envelope on sign-out, and PARSE-REJECTs any route-level `typed_per_filter_config` entry at HCM-parse-time) as the THIRTEENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2). It is the THIRD CONSECUTIVE §9 row to REUSE-by-absence the per-route discipline (after phase 18 + phase 19 — strictly stronger than the prior two phases' 5th-canonical REUSE per ADR-0180 §Decision + phase-20 SPEC §5.4 — the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all per §20.P7 RATIFIED). **9 anchored ADRs + 2 §Decision-touchpoints at phase 20** (ADR-0177 NEW `internal/httpclient/` framework primitive + ADR-0178 NEW `internal/sdsfile/` framework primitive + ADR-0179 5-input HMAC composition + ADR-0180 state-machine + listener-scoped + deny-path + REUSE-by-absence-classification + ADR-0181 5-cookie envelope + 6-counter stat surface + ADR-0182 AES-256-CBC token encryption + ADR-0183 refresh-token rotation + ADR-0184 sign-out + ADR-0185 token_endpoint POST body templates) + **2 IN-PLACE §Decision AMENDMENTs** (ADR-0150 jwks Fetcher refactor + ADR-0159 extauthz httpAuthClient refactor with §Future Work CLOSURE-AT-PHASE-20 paragraph — the FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer per phase-20 SPEC §9 item 1). **D11 hypothesis HELD** at phase-20 IMPL phase-done — ADR-0186 stays unconsumed (no impl-time-unanticipated ADR fired).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.oauth2.v3.OAuth2 → OAuth2Config` (~17 fields consumed) + `OAuth2Credentials` (4 of 5; basic_auth PARSE-REJECT) + `CookieNames` (5 of 7; oauth_nonce + code_verifier parsed-but-not-honored):**

| Proto field | envoy-go phase-20 disposition |
|---|---|
| `token_endpoint` (`HttpUri`) | CONSUMED. `uri` parsed to `*url.URL` at parse time; `cluster` is the upstream cluster the POST routes through; `timeout` SILENT-IGNORED (cluster-manager / `internal/httpclient/` zero-retry default applies per §20.P1 RATIFIED + ADR-0177; the operator-explicit `OAuth2Config.retry_policy` field is DEFERRED per §2.10). |
| `authorization_endpoint` | CONSUMED as string verbatim; concatenated with the RFC-6749 §4.1.1 query envelope at category-(a) 302 emission time per §4.4. |
| `redirect_uri` | CONSUMED. PARSE-REJECT on empty. The post-callback-success category-(b) 302 emits `Location: <redirect_uri>` verbatim (the operator-registered post-sign-in landing page). |
| `redirect_path_matcher` (`PathMatcher`) | CONSUMED. Matches inbound `:path` to identify the callback flow (dispatcher priority 2 per §6.3). |
| `signout_path` (`PathMatcher`) | CONSUMED. Matches inbound `:path` to identify the sign-out flow (dispatcher priority 1 per §6.3); on match emits category-(c) 302 with Max-Age=0 on all 5 envelope cookies. |
| `forward_bearer_token` (bool; OAuth2Config field 7) | CONSUMED per AMEND-6 C3 (~10 LoC). Default false per proto. When true on the valid-envelope path, an `Authorization: Bearer <decrypted-access-token>` header is injected before `ContinueDecoding`. |
| `preserve_authorization_header` (bool) | CONSUMED per §2.15. When false (proto default), a pre-existing inbound `Authorization` header is stripped before envelope evaluation; when true, the inbound header is preserved. |
| `pass_through_matcher` ([]`HeaderMatcher`) | CONSUMED. Any-match short-circuits envelope evaluation; the request bypasses oauth2 entirely + `oauth_passthrough` counter increments per §4.6. |
| `deny_redirect_matcher` ([]`HeaderMatcher`) | CONSUMED. Integrated with the sign-out flow per ADR-0184; informs the category-(c) 302 Location when sign-out matches a deny-redirect entry. |
| `disable_token_encryption` (bool) | CONSUMED per §2.15 + ADR-0182. When true, cookie payloads stored as plaintext (the encrypt/decrypt paths short-circuit); HMAC validation still applies. Sole switch — envoy-go has no runtime-features layer per S2; the upstream `envoy.reloadable_features.oauth2_encrypt_tokens` reloadable-features gate is NOT modeled. |
| `use_refresh_token` (bool) | CONSUMED per §2.15 + ADR-0183. When true, an expired BearerToken + valid RefreshToken arrival triggers the silent refresh-token rotation path; when false, the same arrival path falls through to category-(a) 302 re-challenge. |
| `default_expires_in` + `default_refresh_token_expiry` (`Duration`) | CONSUMED per §6.2. Applied to the cookie-`Max-Age` envelope when the upstream token response omits an `expires_in` field. |
| `auth_scopes` + `resources` ([]string) | CONSUMED. Composed into the category-(a) 302 query envelope per RFC 6749 §4.1.1 + RFC 8707 `resource` repetition per §4.4. |
| `OAuth2Credentials.client_id` (string) | CONSUMED. Embedded in the auth-code + refresh-token POST body templates per AMEND-5. PARSE-REJECT on empty. |
| `OAuth2Credentials.token_secret` (`SdsSecretConfig`) | CONSUMED via `internal/sdsfile/Watcher` per ADR-0178. Reads `generic_secret.inline_string` ONLY (the inner `secret_file` arm PARSE-REJECTs per §8 item 14); ~100ms debounce; `atomic.Pointer[[]byte]` swap for concurrent reader safety. |
| `OAuth2Credentials.hmac_secret` (`SdsSecretConfig`) | CONSUMED via the same `internal/sdsfile/Watcher` primitive. The current bytes feed both the 5-input HMAC composition per AMEND-2 AND the AES-256-CBC KDF (`SHA-256(hmacSecret)[:32]`) per AMEND-1. |
| `OAuth2Credentials.basic_auth` (BASIC_AUTH `client_secret_basic`) | PARSE-REJECT permanently per §2.3 + AMEND-5. envoy-go MVP uses `client_secret_post` exclusively (the 4-field auth-code template embeds `client_secret` in the POST body). |
| `OAuth2Credentials.cookie_domain` (OAuth2Credentials field 5 per AMEND-6 C1) | DEFERRED per §20.P2 RATIFIED. MVP emits host-only cookies (no `Domain=` attribute). The empty default carries the **host-only cookie invariant** load-bearing for the HMAC `domain` input alignment subtlety (see "HMAC `domain` empty-string subtlety" below). |
| `CookieNames.{bearer_token, oauth_hmac, oauth_expires, refresh_token, oauth_id_token}` (5 of 7) | CONSUMED. The 5-cookie envelope keys; default names per the proto's documented defaults; operators may override. Note: `oauth_id_token` is parsed but the cookie is neither emitted nor honored at MVP (couples to the id_token deferral at §2.2). |
| `CookieNames.{oauth_nonce, code_verifier}` (2 of 7) | PARSED-BUT-NOT-HONORED per §2.1. Couples to the PKCE deferral; neither cookie is emitted nor consumed at MVP. |
| `OAuth2Config.{use_pkce, code_verifier_token_expires_in}` | PARSE-REJECT permanently at MVP per §2.1 + AMEND-5. PKCE deferred; future-phase activation lands the 5th `code_verifier` field in the auth-code template per AMEND-5 + ADR-0185 §Decision. |
| `OAuth2Config.id_token` + `OAuth2Config.end_session_endpoint` | DEFERRED per §2.2 + §2.4. id_token validation requires the JWKS round-trip (ADR-0150 jwks NOT consumed by oauth2 at phase 20 — the post-AMENDMENT refactor refactors the EXISTING jwt_authn consumer; phase 20 does not add a NEW jwks consumer). |
| `OAuth2Config.cookie_configs` (`*CookieConfigs` wrapper per AMEND-6 C2) | DEFERRED per §2.5. MVP uses listener-default Set-Cookie attributes (`Path=/; Secure; HttpOnly; SameSite=Lax`). The `Partitioned` cookie attribute (CHIPS-style) also DEFERRED per AMEND-7 (depends on `cookie_configs`; deferred per §8 item 15). |
| `OAuth2Config.{disable_id_token_set_cookie, disable_access_token_set_cookie, disable_refresh_token_set_cookie}` | DEFERRED per §2.6. MVP always emits BearerToken + RefreshToken cookies (the latter when `use_refresh_token=true`); IdToken cookie never emitted. |
| `OAuth2Config.csrf_token_expires_in` (`Duration`) | DEFERRED per §20.P12 + §2.7. MVP uses proto-default 600s (10 minutes) via proto-default fall-through (the field is parsed but its value is ignored when zero; never overridden). |
| `OAuth2Config.retry_policy` (`*RetryPolicy`) | DEFERRED per §20.P1 + §2.10. MVP `internal/httpclient/` applies the zero-retry default (matches upstream wire behavior). Options struct leaves `RetryPolicy` field present-but-unused so a future operator-ergonomics phase wires it without a Client signature break. |

**SDS `core.ConfigSource` oneof dispositions per ADR-0178 + §3.2:**

| oneof arm | phase-20 disposition |
|---|---|
| `PathConfigSource` (oneof field 8 wrapper; non-deprecated; wraps `{path, watched_directory}`) | CONSUMED per §20.P6 RATIFIED. The outer Secret-proto JSON/YAML file watched via fsnotify. |
| `core.ConfigSource.path` (deprecated field 1) | PARSE-REJECT envoy-go-strict per §2.11 + §20.P6. |
| `ApiConfigSource` + `Ads` oneof arms | PARSE-REJECT permanently per §2.11. Out-of-envelope (no in-tree xDS substrate at phase 20). |
| `generic_secret.secret_file` (inner indirect arm) | PARSE-REJECT per §8 item 14. The framework watches the outer Secret-proto JSON/YAML file; the inner double-indirect loading is not modeled at MVP. |
| `generic_secret.inline_string` | CONSUMED — the only arm consumed. Both `token_secret` + `hmac_secret` MUST point at outer Secret-proto files containing inline-string bytes. |

#### Sign-in flow wire shape — category (a) 302 challenge + callback completion (per ADR-0180 + ADR-0185 + §4.4 + §4.5 + §6.3)

Unauthenticated request (no valid 5-cookie envelope; not a sign-out / callback / pass-through match) → `decode_headers.go::handleUnauthenticated` emits **category (a) 302 challenge** per §4.5:

- `:status: 302`
- `location: <authorization_endpoint>?response_type=code&client_id=<client_id>&redirect_uri=<redirect_uri>&state=<HMAC-protected-state-cookie-value>&scope=<space-joined auth_scopes>&resource=<...repeating per RFC 8707 resources>`
- `set-cookie: <state_cookie_name>=<HMAC-protected payload>; Path=/; Secure; HttpOnly; SameSite=Lax`
- 5 envelope cookies cleared (Max-Age=0) per §4.5 table row (a)

State-cookie payload byte-exact shape RATIFIED at fixture-0024 scenario (a) per §12 item A3 (epoch-seconds-as-decimal-string for OauthExpires; state-cookie payload shape stabilized at the IMPL Task 12 fixture-capture).

Subsequent callback arrival at `redirect_uri` (GET method only per §2.14 — POST callbacks PARSE-REJECT at the callback dispatch per the `response_mode=form_post` OAuth-extension envoy-go-strict departure) → `callback.go::handleCallback` validates the inbound state cookie against the request's `state` query parameter via constant-time HMAC compare; on mismatch emits **category (d) 401** with constant body + `oauth_unauthorized_rq++` per §4.6 + bad-state path of §4.2 + AMEND-3 flow-cookie cleanup via `addFlowCookieDeletionHeaders(headers, flow_id_)`.

On state-cookie validation success → `handleCallback` parks the decode goroutine via `StopIteration` + initiates the outbound POST to `token_endpoint` via `cc.tokenEndpointPoster` (the production closure populated at `buildCompiledConfig` consuming `*httpclient.Client.Do` per ADR-0177). POST body template per AMEND-5 / ADR-0185 (4-field auth-code template; PKCE-gated 5th field absent at MVP):

```
grant_type=authorization_code&code={0}&client_id={1}&client_secret={2}&redirect_uri={3}
```

Each `{i}` substitution flows through the NEW filter-local `urlEncode` helper (NOT `url.PathEscape`; the upstream `:/=&?` charset envelope is byte-distinct per §20.P10 RATIFIED + §12 item A5). Spaces emit as `%20`. Content-Type `application/x-www-form-urlencoded` per the OAuth 2.0 token-endpoint convention.

On 2xx response → `applyTokenEndpointResponse` parses the JSON body (`access_token` + `refresh_token` + `id_token` + `expires_in`); encrypts the access + refresh tokens via AES-256-CBC per ADR-0182; computes the 5-input HMAC per AMEND-2 + ADR-0179; emits **category (b) 302 post-callback-success** per §4.5 table row (b) with the populated 5-cookie envelope (BearerToken AES-CBC ciphertext; OauthHMAC base64URL-raw HMAC; OauthExpires epoch-seconds; IdToken omitted at MVP; RefreshToken AES-CBC ciphertext when `use_refresh_token=true`) + `Location: <redirect_uri>` + `oauth_success++` per §4.6.

On non-2xx response → `applyTokenEndpointResponse` classifies per §4.7 + AMEND-3 deny-path simplification (envoy-go-strict): **5xx retry-eligible → category (a) 302 challenge** (NO counter increment; the upstream "redirectToOAuthServer-retry" semantic simplifies to "re-authenticate from scratch"); **4xx terminal → category (d) 401** with constant body `"OAuth flow failed."` + flow-cookie deletion + `oauth_failure++`. Transport-error / malformed-JSON / nil-poster fail-safe routes to category (a) per AMEND-3 retry-eligible classification.

The full disposition matrix is exercised end-to-end at fixture-0024 scenarios (a) + (g) + (h) per §7.1.

#### Refresh flow wire shape — silent rotation (per ADR-0183 + §6.3 + §4.6)

Valid 5-cookie envelope path with **expired BearerToken + valid RefreshToken** → `decode_headers.go::handleRefresh` parks the decode goroutine + initiates the refresh-token POST. Body template per AMEND-5 / ADR-0185 (3-field refresh-token template):

```
grant_type=refresh_token&refresh_token={0}&client_id={1}&client_secret={2}
```

On 2xx response → CONTINUE with **deferred Set-Cookie envelope emission** (the new envelope rides the upstream's response Set-Cookie path via `EncodeHeaders` deferral per ADR-0183 §Decision); `oauth_refreshtoken_success++` per §4.6. The request proceeds upstream with the freshly rotated tokens; no client-visible 302.

On non-2xx response → falls through to **category (a) 302 challenge** per §4.7 + AMEND-3; `oauth_refreshtoken_failure++` per §4.6 (the §4.1 category-(a) trigger row records this as the refresh-failure leg).

**Concurrent-request disposition (per ADR-0183 §Decision):** no per-stream serialization; the latest deferred Set-Cookie wins (the upstream's response Set-Cookie ordering decides). Race-tested at IMPL Task 8 `TestRefreshTokenRotation_Concurrent_*` group with zero data-race violations.

#### Sign-out flow wire shape — full envelope clearing (per ADR-0184 + §4.5 + §6.9)

Inbound `:path` match against `signout_path` (dispatcher priority 1 per §6.3) → `signout.go::handleSignout` emits **category (c) 302 sign-out** per §4.5 table row (c):

- `:status: 302`
- `location:` per `deny_redirect_matcher` integration (when sign-out matches a deny-redirect entry; otherwise empty for browser-default)
- `set-cookie: <each-of-5-envelope-cookies>=; Max-Age=0; Path=/; Secure; HttpOnly; SameSite=Lax` × 5

**No separate `signout_completed` counter per AMEND-4 + S5 + §20.P11 RATIFIED-AS-ABSENT** — the sign-out-completed event IS the 302 emission (the BRAINSTORM-hypothesized 8th counter REFUTED at §20.P8 empirical scrape; envoy-go-strict departure flag CLOSED).

#### Pass-through wire shape (per §6.3 + §4.6)

Inbound headers match any `pass_through_matcher` entry (dispatcher priority 3 per §6.3) → `decode_headers.go::handlePassThrough` short-circuits envelope evaluation; the request bypasses oauth2 entirely (NO 302, NO cookie emission, NO upstream Authorization injection); `oauth_passthrough++` per §4.6.

#### Cookie envelope discipline (per ADR-0181 + §6.4 + §6.5 + §4.5)

The 5-cookie envelope (BearerToken / OauthHMAC / OauthExpires / IdToken-deferred / RefreshToken) emits with **MVP-default Set-Cookie attributes RATIFIED at fixture-0024 scenario (a) per §12 item A2**:

```
Path=/; Secure; HttpOnly; SameSite=Lax
```

NO `Domain=` attribute at MVP per §20.P2 RATIFIED + AMEND-6 C1 (the `OAuth2Credentials.cookie_domain` field is parsed-but-deferred; empty default carries the **host-only cookie invariant**). NO `Partitioned` attribute per AMEND-7 (depends on `cookie_configs`; deferred per §8 item 15).

**HMAC composition per AMEND-2 + ADR-0179 + S4** (5-input newline-joined; emit Base64; accept BOTH Base64 + HexBase64 on read for dual-encoding tolerance):

```
HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))
```

`domain` is the cookie's host scope; `expires` is the OauthExpires value; `token` is the (possibly-encrypted) BearerToken cookie value; `id_token` + `refresh_token` participate as empty strings when absent.

**HMAC `domain` empty-string subtlety** (discovered + recorded at Task 12 follow-up): the callback emit-site at `emitCategoryB_PostCallbackLocked` uses `domain=""` as the HMAC input because the upstream-bound redirect carries no authority context to anchor against. Subsequent validation-site requests to the cookie's host produce the same `domain=""` HMAC input **only when the cookie is host-scoped** (the default per §20.P2 host-only-when-empty). When operators eventually configure a non-empty `cookie_domain` (after the §2.9 deferral lands), the validation site's non-empty `domain` will NOT match the emit-site's `domain=""` HMAC and the cookie envelope will fail HMAC validation. This is a downstream-of-cookie-domain alignment concern carried forward to the future cookie-domain-enabling phase (closure mechanisms: (1) thread the inbound request authority through to the callback emit-site, or (2) inline the HMAC `domain` input as the redirect_uri's host parsed once at parse-time). At MVP the default config (empty `cookie_domain`, host-only cookies) is the only validated path.

**AES-256-CBC token encryption per AMEND-1 + ADR-0182** — `disable_token_encryption=false` (default) emits the BearerToken + RefreshToken cookie values as `Base64URL(IV ‖ CT)` envelopes; `disable_token_encryption=true` skip-path emits plaintext. Key derivation: `SHA-256(hmac_secret)[:32]` → 32-byte AES-256 key. IV: random 16 bytes per encryption (prepended). PKCS#7 padding. **Decryption-failure fall-back per AMEND-3** (returns ciphertext-as-plaintext; downstream HMAC validation rejects naturally; NO `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT).

#### token_endpoint POST body template byte-exactness (per ADR-0185 + AMEND-5 + §20.P10)

The auth-code (4-field; MVP) + refresh-token (3-field) templates are byte-exact upstream per §20.P10 RATIFIED:

```
auth-code:     grant_type=authorization_code&code={0}&client_id={1}&client_secret={2}&redirect_uri={3}
refresh-token: grant_type=refresh_token&refresh_token={0}&client_id={1}&client_secret={2}
```

PKCE-gated 5th field `&code_verifier={4}` absent at MVP (gated per S3 + §2.1; future-phase activation lands the 5th field per ADR-0185 §Decision).

The NEW filter-local `urlEncode` helper at `oauth_client.go` percent-encodes the `:/=&?` charset envelope per §20.P10 + §12 item A5 (NOT stdlib `url.PathEscape`; the byte-exact behavior is different; spaces emit as `%20`). The full charset behavior is vector-tested at IMPL Task 10 unit-tests + fixture-0024 scenario (a) token_endpoint POST body byte-comparison.

#### Stat surface + Prometheus rendering (per ADR-0181 + AMEND-4 + §11 §20.P8)

**6 counters wire-exact upstream** per §20.P8 REFUTED (BRAINSTORM over-counted at 8 + 94-total-names; the empirical scrape reduced the surface to 6 + 92-total-names per AMEND-4) — see `## Stat-name mapping` for the full table extension. Internal stat path: `http.<HCM_stat_prefix>.oauth2.<counter>` — HCM-rooted SN2-reuse per ADR-0143 + ADR-0181 (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18.1+18.2 ext_authz / phase-19.1+19.2 ext_proc; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule; NO new tag-extractor. All 6 counters registered UNCONDITIONALLY at `New()` time per the phase-17/18/19 STRUCTURALLY-UNREACHABLE-counter unconditional-registration discipline. **ABSENT counters per §20.P11 RATIFIED-AS-ABSENT + AMEND-4**: `signout_completed` (the sign-out-completed event IS the category-(c) 302 emission); `cookie_decrypt_failure` (the decryption-failure fall-back returns ciphertext-as-plaintext; downstream HMAC validation rejects naturally — no separate counter). Stat surface 86 → 92 names at the phase-20 phase-done extension.

#### Per-route discipline — REUSE-by-absence (per ADR-0180 + phase-20 SPEC §5)

`OAuth2Config` has **NO `OAuth2PerRoute` message at all** in the v1.37.x proto per §20.P7 RATIFIED — strongest-form evidence (the proto file has no per-route-override message arm). Listener-scoped only.

Per ADR-0110 single-chokepoint discipline + the existing HCM TPFC-placement validation gate: any oauth2 `typed_per_filter_config` (TPFC) entry at route or virtualHost level PARSE-REJECTs at HCM-parse-time with byte-stable error message (consistent with the other listener-scoped filter PARSE-REJECT messages). The `RegisterPerRouteValidator` factory method is the registration hook.

**No ADR-0125 amendment paragraph at phase 20** — **THIRD CONSECUTIVE §9 family-row** to NOT extend the ADR-0125 roster (after phase 18 ext_authz REUSED 5th canonical + phase 19 ext_proc REUSED 5th canonical). Phase 20's REUSE-by-absence is a **STRONGER form of the lesson** than the prior two phases' 5th-canonical REUSE — there is no per-route surface at all, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT discipline rather than a roster-REUSE classification. The ADR-0125 roster does NOT grow monotonically; phase 20 strengthens the lesson WITHOUT amendment (the absence itself is the lesson). ADR-0180 §Decision records the explicit no-amendment classification.

All 6 counters HCM-rooted (no per-route qualifier) per AMEND-4 + S5. Stat-name shape: `http.<HCM_stat_prefix>.oauth2.<counter>`.

#### envoy-go-strict departures (2 — per phase-20 SPEC §13.C.7 + §13.C.8)

**(1) `token_endpoint` POST non-2xx retry-eligible → 302 challenge simplification** per §4.7 + AMEND-3 + ADR-0180 §Decision. Upstream Envoy v1.37.2's `redirectToOAuthServer-retry` semantic loops the user through an additional 302 hop with a `retry_with_oauth_server=true` query parameter; envoy-go simplifies this to a single category-(a) 302 challenge (re-authenticate from scratch — the operator-observable outcome is equivalent for the OAuth 2.0 callback flow's authorization-code grant; no counter delta on the retry-eligible leg). Terminal 4xx still emits category-(d) 401 with constant body. **NO 500 emissions anywhere** in phase 20 per AMEND-3 + §20.P9 (the upstream non-2xx-terminal-500 path is collapsed to the 401-with-constant-body path per AMEND-3). Verified at §15 acceptance item 11 + fixture-0024 scenarios (g) + (h).

**(2) POST callback method PARSE-REJECT** per §2.14 + §20.P3 + ADR-0180 §Decision. The `response_mode=form_post` OAuth-extension variant (where the authorization server POSTs the callback to the redirect_uri instead of GETting it) PARSE-REJECTs at the callback-flow dispatch in `DecodeHeaders` (GET-only at MVP per the canonical-OAuth-2.0 sign-in-flow convention). Future-phase activation of the form_post variant lands an additional dispatcher branch; the GET-only MVP shape is permanently retained as the canonical primary path.

### envoy.filters.http.adaptive_concurrency

Phase 21 ships `envoy.filters.http.adaptive_concurrency` (Envoy v1.37.2 canonical Gradient-1 adaptive-concurrency filter — estimates `minRTT` from sampled request RTTs and continuously adjusts a per-HCM-instance concurrency limit to bound tail latency under load; emits a 503 on overflow with byte-pinned wire shape per AMEND-6) as the FOURTEENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2, oauth2 @ 20). FOURTH CONSECUTIVE §9 family-row to REUSE-by-absence the per-route discipline (the v1.32.4 / v1.37.x adaptive_concurrency proto has NO `AdaptiveConcurrencyPerRoute` message at all per SPEC §5.4 — STRONGER form than the phase-18/19/20 5th-canonical REUSE). **2 anchored ADRs at phase 21** (ADR-0186 Gradient-1 controller state machine + line-cited algorithmic lemmata against `gradient_controller.cc` + inline `Clock` seam + sorted-slice percentile aggregation; ADR-0187 RTDS `enabled.RuntimeFeatureFlag` deferral PARSE-REJECT + enabled-default-OFF semantics per AMEND-4) + **1 IN-PLACE §Decision AMENDMENT** (ADR-0059 float-valued-gauge int64 encoding convention per AMEND-7 — ns for time-typed envoy-go-strict departure; ×1000 for ratio-typed; 0/1 for bool-typed via NEW `internal/stats/conv.go` `BoolToInt` helper). **LEANEST framework-delta §9 row to date** — ZERO new `internal/` framework primitives (the inline `Clock` seam stays in-package per the phase-17 EXTRACT-NOW-only-when-trigger-fires lesson). **D8 hypothesis HELD** at phase-21 IMPL phase-done — ADR-0188 + ADR-0189 stay UNCONSUMED (STRENGTHENED two-slot escape-valve buffer; no impl-time-unanticipated ADR fired).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency` + nested `GradientControllerConfig` + `ConcurrencyLimitCalculationParams` + `MinimumRTTCalculationParams`:**

| Proto field | envoy-go phase-21 disposition |
|---|---|
| `AdaptiveConcurrency.concurrency_controller_config` (oneof) | CONSUMED for the `gradient_controller_config` arm ONLY (the only arm at upstream v1.37.x). Any non-Gradient alternative PARSE-REJECTs. |
| `AdaptiveConcurrency.enabled` (`core.RuntimeFeatureFlag`) | PARTIAL per ADR-0187 — `enabled.default_value` honored (static-default OFF per AMEND-4); `enabled.runtime_key != ""` triggers HCM-parse-time PARSE-REJECT (RTDS runtime keying DEFERRED). |
| `GradientControllerConfig.sample_aggregate_percentile` (`type.Percentile`) | CONSUMED — bounds [0.0, 100.0]; default 50.0 per upstream proto. Feeds the sorted-slice `Quantile` helper at sample-aggregation tick. |
| `GradientControllerConfig.concurrency_limit_params` (`ConcurrencyLimitCalculationParams`) | CONSUMED — sub-fields below. |
| `GradientControllerConfig.min_rtt_calc_params` (`MinimumRTTCalculationParams`) | CONSUMED — sub-fields below. |
| `ConcurrencyLimitCalculationParams.max_concurrency_limit` (`UInt32Value`) | CONSUMED — upper bound on the per-HCM-instance concurrency limit; default 1000 per upstream proto. |
| `ConcurrencyLimitCalculationParams.concurrency_update_interval` (`Duration`) | CONSUMED — PARSE-REJECT on missing/zero per SPEC §5.3. The fixed-period concurrency-update tick interval. |
| `MinimumRTTCalculationParams.interval` (`Duration`) | CONSUMED — PARSE-REJECT on missing/zero per SPEC §5.3. The minRTT recalc window interval. |
| `MinimumRTTCalculationParams.request_count` (`UInt32Value`) | CONSUMED — minimum samples required during the minRTT recalc window; default 50 per upstream proto. |
| `MinimumRTTCalculationParams.jitter` (`type.Percent`) | CONSUMED per AMEND-2 C2 — additive-to-next-interval-delay (NOT a recalc-trigger probabilistic gate); default 15.0% per upstream proto. |
| `MinimumRTTCalculationParams.min_concurrency` (`UInt32Value`) | CONSUMED — concurrency limit pinned to this value during the minRTT recalc window; default 3 per upstream proto. |
| `MinimumRTTCalculationParams.buffer` (`type.Percent`) | CONSUMED — multiplicative inflation factor applied to the recalculated minRTT before publishing; default 25.0% per upstream proto. |
| `MinimumRTTCalculationParams.fixed_value` (`Duration`) | DEFERRED + PARSE-REJECT per SPEC §5.3 + ADR-0186 §Consequences (d). MVP requires `interval`. v1.32.4 proto-binding limitation: the field is NOT exposed in the v1.32.4 Go binding (only v1.37.0+); PARSE-REJECT wording is preserved but the arm is structurally unreachable until a proto-bump phase. |
| `AdaptiveConcurrency` (any `typed_per_filter_config` entry at route or virtualHost level) | PARSE-REJECT at HCM-parse-time — the v1.32.4 / v1.37.x adaptive_concurrency proto has NO `AdaptiveConcurrencyPerRoute` message at all per SPEC §5.4. REUSE-by-absence (FOURTH CONSECUTIVE §9 row to skip the per-route surface). |

#### Controller state machine (per ADR-0186 + AMEND-2 + line-cited lemmata against `gradient_controller.cc`)

- **First-tick semantics (per AMEND-2 C4):** `New()` records `lastSampleAggregation = startTime` + `minRTTCalculationActive = true` + `concurrencyLimit = min_concurrency`; the first concurrency-update tick fires `concurrency_update_interval` after `startTime`; the first minRTT recalc window completes `interval` after `startTime` (one continuous window from boot — no pre-roll grace period).
- **Concurrency-update tick (per SPEC §4.2 + ADR-0186):** every `concurrency_update_interval`, the controller aggregates the sample buffer via the sorted-slice `Quantile` helper at `sample_aggregate_percentile`, computes the per-tick gradient bounded to `[0.5, 2.0]` (`minRTT / sampleRTT`), publishes new concurrency limit as `ceil(currentLimit × gradient)` bounded to `[min_concurrency, max_concurrency_limit]`, and resets the sample buffer.
- **minRTT recalc (per SPEC §4.5 + AMEND-2 C1):** during the active recalc window, all sampled RTTs aggregate via percentile (NOT MIN per AMEND-2 C1 — the upstream `gradient_controller.cc` uses the configured `sample_aggregate_percentile`, not a hard minimum); on window close, the percentile-aggregated value × `(1 + buffer)` publishes as the new minRTT and the active flag clears.
- **5-consec-min forced-recalc trigger (per AMEND-2 C3):** after 5 consecutive concurrency-update ticks at `min_concurrency`, the controller force-triggers a minRTT recalc window (the assumption is that the published minRTT has drifted stale; this matches upstream `gradient_controller.cc:284-296` per ADR-0186).
- **Jitter (per AMEND-2 C2):** additive-to-next-interval-delay — at each recalc-window-close, the next window's start is delayed by `Uniform(0, interval × jitter)`; NOT a per-trigger probabilistic gate (envoy-go-strict alignment with the upstream cc-pinned semantic).

#### 7-name stat surface + Prometheus rendering

7 names total — 1 counter + 6 gauges — namespace `http.<HCM_stat_prefix>.adaptive_concurrency.gradient_controller.<stat>` HCM-rooted per AMEND-3 C2 (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18.1+18.2 ext_authz / phase-19.1+19.2 ext_proc / phase-20 oauth2). All 7 unconditionally active under default MVP config (NONE structurally unreachable — continues the phase-20 pattern). Stat surface 92 → 99 names at the phase-21 phase-done extension. NO new SN flattening rule; SN2-reuse RATIFIED at SPEC §11 §21.P3 PARTIAL. See `## Stat-name mapping` for the full table extension.

**Encoding convention per ADR-0059 §Decision AMENDMENT (per AMEND-7):**

- `rq_blocked` — counter; increments per Block emission from `forwardingDecision()` CAS-loop overflow (per AMEND-6).
- `concurrency_limit` — int64 raw uint32 gauge.
- `gradient` — int64 ×1000 gauge (bounded `[500, 2000]` over the `[0.5, 2.0]` domain; gives 3 decimal places of precision).
- `burst_queue_size` — int64 signed gauge (sqrt of `currentLimit × gradient`; per SPEC §4.4 semantics correction from BRAINSTORM §5.1).
- `sample_rtt_msecs` — int64 nanoseconds gauge (envoy-go-strict departure; stat NAME preserves byte-exact upstream).
- `min_rtt_msecs` — int64 nanoseconds gauge (envoy-go-strict departure; stat NAME preserves byte-exact upstream).
- `min_rtt_calculation_active` — int64 0/1 gauge via NEW `internal/stats/conv.go` `BoolToInt` helper (1 during the minRTT recalc window).

#### Deny-path wire shape — 503 overflow (per AMEND-6 + §21.P-D2 RATIFIED)

Overflow path (`forwardingDecision()` CAS-loop fails to acquire a slot under the current concurrency limit) → byte-pinned 503 response:

- `:status: 503`
- `content-type: text/plain`
- `content-length: 25`
- Body: `reached concurrency limit` (25 bytes verbatim; NO trailing newline)

The `rq_blocked` counter increments per Block emission. Stat surface byte-exactness is the only cross-side axis exercised at fixture-0025 scenario (b) per the Pragmatic-middle Q1 posture; algorithmic divergence (sorted-slice vs CircllHist; up to 1 bin-width at percentile boundaries) is explicitly accepted per BRAINSTORM §8 item 4.

#### Per-route discipline — REUSE-by-absence (per SPEC §5.4 + ADR-0186 §Consequences)

`AdaptiveConcurrency` has **NO `AdaptiveConcurrencyPerRoute` message at all** in the v1.32.4 / v1.37.x proto per SPEC §5.4 — strongest-form evidence. Listener-scoped only.

Per ADR-0110 single-chokepoint discipline + the existing HCM TPFC-placement validation gate: any adaptive_concurrency `typed_per_filter_config` (TPFC) entry at route or virtualHost level PARSE-REJECTs at HCM-parse-time with byte-stable error message (consistent with the other listener-scoped filter PARSE-REJECT messages; mirrors phase-20 oauth2's REUSE-by-absence enforcement).

**FOURTH CONSECUTIVE §9 family-row to NOT extend the ADR-0125 roster** (after phase 18 + phase 19 + phase 20). Phase 21's REUSE-by-absence is the **STRONGER form of the lesson** as for phase 20; ADR-0125 roster does NOT grow monotonically. ADR-0186 records the explicit no-amendment classification. See `## Per-route canonical patterns cross-reference` below for the phase-21 cross-reference paragraph.

**envoy-go-strict departure — RTT-gauge units (per phase 21 SPEC §13 item C4 + AMEND-3 C3):** envoy-go encodes `sample_rtt_msecs` and `min_rtt_msecs` as int64 **nanoseconds** via `Gauge.Set(d.Nanoseconds())`; upstream Envoy v1.37.2 encodes them as int64 **milliseconds** via `duration_cast<milliseconds>` at `gradient_controller.cc:75-76, 154-155`. Stat NAMES preserve byte-exact upstream (`sample_rtt_msecs` + `min_rtt_msecs` — note the `_msecs` suffix is part of the upstream `ALL_GRADIENT_CONTROLLER_STATS` macro and is preserved as the name even though envoy-go's encoded value is nanoseconds). Per-metric `# HELP` text disambiguates the unit so operators consulting both proxies see the same stat name with explicit-unit context. Documented load-bearing for future cross-side scrape parity work (currently DEFERRED to a future cross-side fixture-0025-extension phase per phase-21 §8 item 2).

**envoy-go-strict departure — percentile aggregation algorithm (per phase 21 SPEC §13 item C5 + BRAINSTORM §8 item 4):** envoy-go uses **sorted-slice percentile aggregation** (`internal/filter/http/adaptive_concurrency/percentile.go::Quantile(samples []time.Duration, p float64) time.Duration` — sort + index interpolation; ~50-80 LoC). Upstream Envoy v1.37.2 uses **CircllHist** (log-linear histogram with ~4% bin precision per `gradient_controller.h:19, 288-289`). Numeric divergence is ≤ 1 bin-width at percentile boundaries: gradient values + new-limit values + sampled-RTT values may differ cross-side by up to one CircllHist bin. Phase 21 explicitly accepts this divergence per the Pragmatic-middle Q1 posture: the only cross-side byte-exact axis is the 503-overflow wire shape (per AMEND-6 + §21.P-D2 RATIFIED). A future algorithmic-fidelity-extension phase could swap to CircllHist if cross-side byte-exact algorithmic parity becomes load-bearing.

**envoy-go-strict departure — `response_code_details` NOT emitted** per phase-21 SPEC §8 item 8 + §12 item A3: envoy-go MVP has no access-log surface — the 503 deny path's pseudo-`response_code_details = "reached_concurrency_limit"` lives only in code comments. Joint divergence-window with phases 16/17/18/19/20 — see `### Phase 21 forward-pointer notes` below.

See `### Phase 21 forward-pointer notes` below for the 8 SPEC §8 deferred items + the D8-HELD confirmation + the fixture-0025 cross-side promotion forward-pointer.

### envoy.filters.http.lua

Phase 22.1 ships `envoy.filters.http.lua` (Envoy v1.37.2 canonical HTTP Lua scripting filter — operator-supplied Lua source code dispatches per-stream into a sandboxed gopher-lua VM, hooks `envoy_on_request` + `envoy_on_response` against bridge methods for headers manipulation + logging + `:streamInfo()` subset access + synchronous `:respond()` short-circuit) as the FIFTEENTH §9 family-row (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2, oauth2 @ 20, adaptive_concurrency @ 21). FIRST §9 family-row to (i) **delegate per-request behavior to operator-authored interpreted scripts**, (ii) introduce a NEW framework primitive of substantial scope since phase 17 jwt_authn (`internal/lua/` per ADR-0188; ENDS the phase-21 ZERO-NEW-framework-primitive streak), and (iii) introduce a third-party Lua VM dependency (`github.com/yuin/gopher-lua` v1.1.2 — pure-Go Lua 5.1; MIT-licensed; NO CGO). **2 anchored ADRs at phase 22.1** (ADR-0188 NEW `internal/lua/` framework primitive — VM lifecycle + per-stream `*LState` + per-script-source `*Chunk` compile cache + `SandboxConfig` zero-value `StrictUpstreamParity` posture per AMEND-1 + EXPLICIT API-REVISION ALLOWANCE for consumer #2; ADR-0189 NEW `internal/filter/http/lua/` package shape — `compiledConfig` + 3-counter `filterStats` + 19-arm PARSE-REJECT roster + 4-arm DataSource + pragmatic-middle bridge 21 entries + fixture-0026 + `BootRejectFixture` + envoy-go-side `"script load error: "` wording-pinning). **D-P10 R6 disposition: STANDS WEAK-default** — per-stream `*LState` construction measured at `ns/op = 69865` (~70µs) << 1ms threshold; ADR-0190 NOT consumed; per-stream construction posture retained (no `*LState`-pool); carries forward to 22.2 BRAINSTORM. **3 envoy-go-strict departure records** land at this 22.1 IMPL bundle: stdlib-sandbox-strict (per AMEND-1); `respond_calls` counter (per AMEND-3 corrected from BRAINSTORM 2-record bundle to 1 record); runtime-error log-message wording (per AMEND-9).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.lua.v3.Lua` + per-route `LuaPerRoute` + `core.DataSource`:**

| Proto field | envoy-go phase-22.1 disposition |
|---|---|
| `Lua.default_source_code` (`core.DataSource`) | CONSUMED — resolved at config-load via 4-arm DataSource arm (Filename + InlineBytes + InlineString + EnvironmentVariable). Resolved bytes compile to a `*lua.Chunk` cached on `*compiledConfig` for filter-instance lifetime. |
| `Lua.inline_code` (`string`) | PARSE-REJECT per parent §6.2 arm 3 + envoy-go-strict deprecated-field-rejection. Byte-stable wording: `"lua: inline_code is deprecated; use default_source_code"`. |
| `Lua.source_codes` (`map<string, core.DataSource>`) | PARSE-REJECT per parent §6.2 arm 4 — multi-script deferred to phase 22.3. Byte-stable wording: `"lua: source_codes map is not yet supported (lands in phase 22.3)"`. |
| `Lua.stat_prefix` (`string`) | CONSUMED. Qualifies the 3-counter stat namespace per AMEND-2 (HCM-rooted `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>`). PARSE-REJECT on `stats.IsValidName` regex violation per arm 19 (`"lua: stat_prefix: invalid characters in %q (must match %s)"`) — arm surfaced by Task 11 fuzzer + matches `hcm/config.go:209` + `cluster/manager.go:205` precedent. |
| `Lua.clear_route_cache` (`BoolValue`) | DEFERRED to 22.2 (no route-cache mutation surface in the 22.1 headers-bridge). |
| `core.DataSource.specifier.filename` | CONSUMED with 16 MiB cap (`io.LimitReader`) per arm 9-extension — surfaced by Task 11 fuzzer (`/dev/full` infinite-read OOM-kill defense). Byte-stable wording: `"lua: default_source_code: file %q exceeds the maximum script size of %d bytes"`. |
| `core.DataSource.specifier.inline_bytes` | CONSUMED. PARSE-REJECT on empty: `"lua: default_source_code: inline_bytes empty"`. |
| `core.DataSource.specifier.inline_string` | CONSUMED. PARSE-REJECT on empty: `"lua: default_source_code: inline_string empty"`. |
| `core.DataSource.specifier.environment_variable` | CONSUMED via `os.LookupEnv`. PARSE-REJECT on name-empty / unset / empty-value. |
| `core.DataSource.watched_directory` | PARSE-REJECT — deferred to a future Runtime/hot-reload phase. Byte-stable wording: `"lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"`. Mirrors phase-21 ADR-0187 RTDS-deferral pattern. |
| `LuaPerRoute` (any `typed_per_filter_config` entry) | PARSE-REJECT at boot-level `RegisterPerRouteValidator` per ADR-0110 single-chokepoint — per-route discipline deferred to 22.3 (NEW 9th canonical per ADR-0125 §(xiv) AMENDMENT). Byte-stable wording: `"lua: per-route configuration is not yet supported (lands in phase 22.3)"`. |

#### 19-arm PARSE-REJECT roster (parent §6.2 + 22.1 IMPL fuzzer extensions)

Per parent SPEC §6.2 + AMEND-4 (CONFIRMS-AND-TIGHTENS to 18 baseline arms) + AMEND-5 (10 DataSource-related leaves) + AMEND-6 (3 additional baseline arms; sandbox-violation arm RETRACTED) + Task 11 fuzzer extensions (arm 19 `Lua.stat_prefix` invalid chars + arm 9-extension `Filename` 16 MiB cap). Byte-stable wording per ADR-0080 + the phase-21 SPEC §5 precedent. Constants live as package-private `parseReject*` consts at `internal/filter/http/lua/compiled_config.go` + `datasource.go`. All wording is byte-pinned via the package's `compiled_config_test.go::TestParseRejectConstants_ByteExactWording` + `datasource_test.go::TestResolveDataSource_ByteExactWording`.

**Roster (19 active arms — was 18 at SPEC time; Task 11 fuzzer surfaced arm 19 + arm 9-extension):**

| Arm | Trigger | Wording (byte-exact) |
|---|---|---|
| 1 | `typed_config` nil | `lua: typed_config required` |
| 2 | `typed_config` Unmarshal failure | `lua: typed_config unmarshal: <inner>` (wrap) |
| 3 | `Lua.inline_code != ""` (deprecated-field) | `lua: inline_code is deprecated; use default_source_code` |
| 4 | `Lua.source_codes` non-empty (deferred to 22.3) | `lua: source_codes map is not yet supported (lands in phase 22.3)` |
| 5 | `Lua.default_source_code` absent | **silent no-op per D1 REFUTED** (see "D1 disposition" below). Reserved constant `parseRejectDefaultSourceCodeRequired = "lua: default_source_code required"` retained in source with `//nolint:unused` for future policy-bump migration per the phase-21 `parseRejectFixedValueDeferred` reserved-constant precedent. |
| 6 | DataSource specifier oneof empty | `lua: default_source_code: specifier oneof required` |
| 7 | DataSource `WatchedDirectory` set | `lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)` |
| 8 | DataSource `Filename` empty | `lua: default_source_code: filename empty` |
| 9 | DataSource `Filename` read failure | `lua: default_source_code: read file %q: <inner>` (wrap) |
| 9-ext | DataSource `Filename` > 16 MiB (Task 11 fuzzer) | `lua: default_source_code: file %q exceeds the maximum script size of %d bytes` (cap = 16777216 bytes; defense vs `/dev/full`-class infinite-read OOM-kill) |
| 10 | DataSource `Filename` zero-byte contents | `lua: default_source_code: file %q is empty` |
| 11 | DataSource `InlineBytes` empty | `lua: default_source_code: inline_bytes empty` |
| 12 | DataSource `InlineString` empty | `lua: default_source_code: inline_string empty` |
| 13 | DataSource `EnvironmentVariable` name empty | `lua: default_source_code: environment_variable name empty` |
| 14 | DataSource `EnvironmentVariable` unset | `lua: default_source_code: environment_variable %q not set` |
| 15 | DataSource `EnvironmentVariable` empty value | `lua: default_source_code: environment_variable %q is empty` |
| 16 | Lua compile failure (gopher-lua `*lua.ApiError`) | `lua: default_source_code: compile: <inner>` (wrap; boot-sink at `cmd/envoy-go/main.go:191` prepends `"script load error: "` per §13-W) |
| 17 | Script defines neither `envoy_on_request` nor `envoy_on_response` | **silent no-op per D1 REFUTED** (see "D1 disposition" below). Reserved constant `parseRejectScriptMissingHooks = "lua: default_source_code: script defines neither envoy_on_request nor envoy_on_response"` retained in source for future policy-bump migration. |
| 18 | Any per-route `LuaPerRoute` (TPFC entry at route or virtualHost) | `lua: per-route configuration is not yet supported (lands in phase 22.3)` — boot-level `RegisterPerRouteValidator` per ADR-0110 single-chokepoint. |
| 19 | `Lua.stat_prefix` violates `stats.IsValidName` regex (Task 11 fuzzer) | `lua: stat_prefix: invalid characters in %q (must match %s)` — pre-emits at config-load; mirrors `hcm/config.go:209` + `cluster/manager.go:205` `stats.IsValidName` pre-check precedent. |

**D1 disposition (REFUTED at Task 2 first-action upstream scrape).** Per 22.1 SPEC §12-D1 + parent §12-D1: empirically scraped upstream Envoy v1.37.2 source via WebFetch against `source/extensions/filters/http/lua/{config.cc,lua_filter.cc}`. Both arms 5 + 17 silently accept the absent / no-hooks case as a no-op pass-through filter. **Arm 5** (`lua_filter.cc:1455-1485` `FilterConfig::FilterConfig`): the `if (has_default_source_code()) { … } else if (!inline_code().empty()) { … }` chain has **no terminal `else` arm**; when both predicates are false, `default_lua_code_setup_` stays null and the filter loads as a silent pass-through. **Arm 17** (`lua_filter.cc:140-181` `PerLuaCodeSetup::PerLuaCodeSetup`): missing-hook branch emits `ENVOY_LOG(info, …)` and falls through; no `throw EnvoyException` fires. envoy-go matches upstream per the byte-equivalent posture: both arms flip to silent-no-op (`compiled_config.go::buildCompiledConfig` short-circuits at `m.GetDefaultSourceCode() == nil` returning `&compiledConfig{chunk: nil}`; runtime hooks at `decode_headers.go` / `encode_headers.go` treat `cc.chunk == nil` as "no script defined → pass through" + per-hook `vm.HasGlobalFunc(name)` check covers the missing-hooks runtime side). Reserved wording constants pinned in source for future policy-bump migration.

#### 4-arm DataSource resolution (per parent §5.3 + §6.2 arms 6-15 + AMEND-5)

The `internal/filter/http/lua/datasource.go::resolveDataSource` helper resolves `core.DataSource` to bytes at config-load time (NOT per-request — the resolved bytes feed into the per-`*compiledConfig` cached `*lua.Chunk` via `internal/lua/CompileScript`). The 4 active arms:

1. **`Filename`** — `os.Open` + `io.LimitReader(f, maxFilenameScriptBytes+1)` (16 MiB cap defense vs `/dev/full`-class infinite-read OOM-kill per Task 11 fuzzer arm 9-extension). PARSE-REJECT on (a) name-empty (arm 8), (b) `os.Open` / `io.Copy` failure (arm 9), (c) read exceeds 16 MiB cap (arm 9-ext), (d) zero-byte contents (arm 10).
2. **`InlineBytes`** — bytes used directly. PARSE-REJECT on empty (arm 11).
3. **`InlineString`** — bytes from string. PARSE-REJECT on empty (arm 12).
4. **`EnvironmentVariable`** — `os.LookupEnv`. PARSE-REJECT on name-empty (arm 13), unset (arm 14), empty value (arm 15).

`WatchedDirectory` PARSE-REJECTs (arm 7) — deferred to future Runtime/hot-reload phase per BRAINSTORM Q5. Empty oneof PARSE-REJECTs (arm 6).

#### Pragmatic-middle bridge surface (21 entries per BRAINSTORM Q6 + this 22.1 SPEC §4.3)

**Per-stream dispatch model:** at `DecodeHeaders` / `EncodeHeaders`, the filter constructs a fresh `internal/lua/VM` (cheap per-stream `*lua.LState` per WEAK-default — Task 12 R6 STANDS at ~70µs/stream, well under 1ms threshold), binds the bridge metatables (5 metatable installs: requestHandle + responseHandle + headers + streamInfo + headersIter), runs the cached `*Chunk` (loads `*FunctionProto` onto the LState — no recompilation), and invokes the hook via `vm.CallGlobal("envoy_on_request", reqUd)` / `vm.CallGlobal("envoy_on_response", respUd)`. The hooks themselves are dispatched only when `vm.HasGlobalFunc(name)` returns true (D1 REFUTED — missing hook silently no-ops per upstream parity).

**Bridge surface roster (21 entries; 2 hooks + 7 headers + `__pairs` + 6 log + 4 streamInfo + 1 respond per-side):**

| # | Entry | Side | Semantic |
|---|---|---|---|
| 1 | `envoy_on_request(rh)` | decode | per-stream invocation if global is defined (D1 REFUTED — silent skip if undefined). `rh` carries the request-side handle userdata. |
| 2 | `envoy_on_response(rh)` | encode | per-stream invocation if global is defined; same semantic as above. `rh` carries the response-side handle userdata. |
| 3 | `rh:headers()` | both | returns the headers-userdata bound to the per-side carrier (`http.Header` for envoy-go). |
| 4 | `headers:get(name)` | both | returns first value as Lua string (or nil if absent). Case-insensitive per Go `http.Header.Get`. |
| 5 | `headers:add(name, value)` | both | appends value to name's list. Mirrors `http.Header.Add`. |
| 6 | `headers:replace(name, value)` | both | replaces all values for name with single value. Mirrors `http.Header.Set`. |
| 7 | `headers:remove(name)` | both | removes name. Mirrors `http.Header.Del`. |
| 8 | `headers:__index(name)` | both | metamethod alias for `:get(name)` (Lua-idiomatic `headers["x-foo"]`). |
| 9 | `headers:__newindex(name, value)` | both | metamethod alias for `:replace(name, value)` (Lua-idiomatic `headers["x-foo"] = "bar"`). |
| 10 | `headers:__pairs()` | both | NEW Lua-side shim — gopher-lua is Lua 5.1 (`pairs()` doesn't auto-dispatch `__pairs` metamethod on userdata; 5.2+ feature). Bridge installs a per-headers `__pairs` userdata-method that snapshots into an alphabetically-sorted `[][2]string` and walks by integer index. Iteration order is **deterministic across runs and across proxies** (alphabetical by key) — RATIFIES D7 RESOLUTION at 22.1 SPEC §11.2 (REFUTES BRAINSTORM hypothesis that headers-map was insertion-ordered; envoy-go's `net/http.Header` is `map[string][]string` and unordered per Go map semantics; bridge snapshot fixes the determinism). |
| 11-16 | `rh:logTrace(msg)` / `rh:logDebug(msg)` / `rh:logInfo(msg)` / `rh:logWarn(msg)` / `rh:logErr(msg)` / `rh:logCritical(msg)` | both | 6 log methods per parent §7.1 — emit via `log.Printf("lua %s: %s", level, msg)`. (envoy-go does not have a multi-level log framework at 22.1; all levels render identically — scope-narrow per BRAINSTORM Q6 pragmatic-middle.) |
| 17 | `rh:streamInfo():protocol()` | both | returns `"HTTP/1.1"` / `"HTTP/2"` per the per-stream protocol detection. |
| 18 | `rh:streamInfo():routeName()` | both | returns the matched route name (empty string if no route matched). |
| 19 | `rh:streamInfo():downstreamLocalAddress()` | both | returns the proxy's local addr the downstream client connected to (e.g., `"127.0.0.1:8080"`). |
| 20 | `rh:streamInfo():downstreamDirectRemoteAddress()` | both | returns the direct downstream peer addr (PROXY-protocol unaware at 22.1; the "direct" qualifier preserves upstream naming so the 22.2 PROXY-protocol-aware extension lands as `:downstreamRemoteAddress` alongside this method). |
| 21 | `rh:respond(headers, body)` | request-side ONLY | byte-pinned per parent §11.6.7 + AMEND-7 + AMEND-8. See "`:respond()` byte-pin" below. Encode-side invocation runtime-rejects with byte-exact `"respond not currently supported in the response path"` per AMEND-8. |

**Deferred bridge surface (raises Lua runtime error per the Lua-idiomatic disposition):** body methods (`:body()`, `:bodyChunks()`, `:trailers()`); metadata methods (`:metadata()`, `:dynamicMetadata()`); `:httpCall()` (consumer #1 of `internal/httpclient/` per phase-20 ADR-0177; lands at 22.2); crypto helpers (`:base64Decode()`, `:sha256()`); filesystem helpers (`:fileBytes()`); `:timestamp()`; `:connection()`; full `:streamInfo()` (route-metadata / cluster-info / SSL-context / dynamic-metadata accessors). All defer to 22.2 IMPL per parent §10 forward-pointers.

#### `:respond()` byte-pin (per parent §11.6.7 + AMEND-7 + AMEND-8)

For `rh:respond({[":status"]="403"}, "denied")` from `envoy_on_request`:

- `:status: 403` (upstream-parity wire status)
- `content-length: 6` (auto-set from body length per upstream `Utility::prepareLocalReply` at `utility.cc:1270`)
- `content-type: text/plain` (default per `utility.cc:1241,1273` because Lua headers table did NOT supply `content-type`)
- Body: `denied` (6 bytes verbatim; no trailing newline; no JSON wrapping)
- No upstream request initiated
- `response_code_details: "lua_response"` (access-log surface only; envoy-go MVP does not emit access logs — joint divergence-window with phases 16/17/18/19/20/21 — see `### Phase 21 forward-pointer notes` above)
- `respond_calls` counter increments by 1 (envoy-go-strict counter per AMEND-3)
- `executions` counter increments by 1 (upstream-parity per AMEND-3 + `lua_filter.cc:872`)

**`:status` validation:** values outside `[200, 600)` runtime-reject with byte-exact `":status must be between 200-599"` per upstream `lua_filter.cc:~578-580`. The error is caught by the panic-recovery wrapper, increments `lua.errors`, and the script aborts (degraded pass-through to upstream).

**Encode-side runtime-reject:** `:respond()` invoked from `envoy_on_response` runtime-rejects with byte-exact `"respond not currently supported in the response path"` per upstream `lua_filter.cc:1031-1034` (`EncoderCallbacks::respond` raises `luaL_error(state, ...)`). The error is caught by the panic-recovery wrapper, increments `lua.errors`, and the script aborts (response continues unmodified to client). Not a PARSE-REJECT — runtime-only.

#### envoy-go-side `"script load error: "` wording-pinning (per parent §13-W + §11.7.5 + AMEND-10)

The arm-16 lua compile-failure error wraps with the byte-stable prefix `"lua: default_source_code: compile: "` at the filter package's `wrapParseRejectScriptCompileFailed` helper. At the boot-sink (`cmd/envoy-go/main.go:191`), the helper `maybeWrapLuaScriptLoadError` detects this substring in the surfaced error chain (`listener: %q: filter_chains[%d]: hcm: http_filters[%d]: factory: <inner>`) and prefixes the rendered string with the upstream-parity `"script load error: "` wrap. Non-lua errors + non-compile lua errors fall through unchanged (the wrap is filter-scoped, not generic — keyed on the byte-stable substring emitted by the filter package). This pins the cross-side substring assertion at fixture-0026 scenario (g) per AMEND-10 option 2 (substring-match `"script load error"` on both reference + subject stderr — REFUTES BRAINSTORM Q9 byte-exact-stderr claim per three independent divergence sources documented at parent §11.7).

#### 3-counter stat surface + Prometheus rendering

3 names total — all counters — namespace `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` HCM-rooted per AMEND-2 + parent §7 + this 22.1 SPEC §8 (mirrors phase-09 fault / phase-12 csrf / phase-14 compressor / phase-16 rbac / phase-17 jwt_authn / phase-18/19 ext_authz+ext_proc / phase-20 oauth2 / phase-21 adaptive_concurrency). 2 upstream-parity (`errors` + `executions` per AMEND-3) + 1 envoy-go-strict (`respond_calls` per AMEND-3 corrected from BRAINSTORM 2-record bundle). All 3 unconditionally active under default MVP config (NONE structurally unreachable — continues the phase-20/21 pattern). Stat surface 99 → 102 names at the phase-22.1 phase-done extension. NO new SN flattening rule; SN2-reuse RATIFIED (HCM-rooted via the existing `envoy_http_conn_manager_prefix` SN2 extractor). See `## Stat-name mapping` for the full table extension.

**envoy-go-strict departure — `respond_calls` counter (per AMEND-3 corrected from BRAINSTORM 2-record bundle):** envoy-go emits `respond_calls` to give operators visibility into `:respond()` short-circuit frequency (how often Lua scripts terminate the request before upstream dispatch). Upstream Envoy v1.37.2 does NOT emit this counter. Collision-free verified at parent SPEC §11.5.3 against the upstream `ALL_LUA_FILTER_STATS` macro (`errors` + `executions` only). Operator-visibility rationale: `:respond()` is a load-bearing short-circuit primitive (auth-deny, rate-limit-deny, geo-block); knowing its rate enables dashboards + alerting that upstream's surface does not afford.

**envoy-go-strict departure — runtime-error log-message wording (per AMEND-9):** gopher-lua formats Lua runtime errors as `[string "chunk"]:line: msg` (per `value.go::String()`); LuaJIT formats them as `chunk:line: msg`. Only the envoy-side log shows the divergent wording (wire never carries error strings — wire shape is byte-exact). Departure documented for operators consulting logs from both proxies side-by-side; cross-side stderr substring assertions at fixture-0026 scenario (g) carve out the divergence via the option-2 substring-match `"script load error"` discipline per AMEND-10.

**envoy-go-strict departure — stdlib-sandbox-strict (per AMEND-1):** envoy-go's `internal/lua/SandboxConfig` zero-value posture is **StrictUpstreamParity** — DENY `io`, `os`, `debug`, `package`, `channel`; ALLOW `base` (selected — `print` redirected to drop-or-logInfo sink via `WithBasePrintSink`), `table`, `string`, `math`, `coroutine` (per AMEND-A4 — `AllowCoroutine` zero-value-defaulted true). Upstream Envoy v1.37.2 calls `luaL_openlibs(state.get())` — full LuaJIT stdlib — WITHOUT subsequent neutering (per `source/extensions/filters/common/lua/lua.cc`). envoy-go's strict default-deny posture is therefore an envoy-go-strict DEPARTURE not parity preservation. Rationale: gopher-lua exposes sandbox-breaking modules (`os.execute` subprocess; `os.exit` terminates host process; `io.popen` shell-out; `package.*` filesystem-search loader; `channel` Go-native chan; `debug.getupvalue`/`setupvalue` cross-closure tampering). Upstream's per-worker LuaJIT VM scoping bounds the blast-radius assumption upstream relies on; envoy-go's per-stream goroutine dispatch model on a SHARED process cannot make the same assumption (any operator-supplied Lua can compromise the entire envoy-go process). Departure documented at the BEHAVIOR_CONTRACT.md envoy-go-strict departure record bundle; opt-out via explicit `WithSandboxConfig(SandboxConfig{...})` per-consumer (the API surface is intentionally explicit — silent broadening requires an operator-visible code edit).

#### Per-route discipline — PARSE-REJECT-by-arm-18 (deferred to 22.3)

`LuaPerRoute` (TPFC entry at route or virtualHost level) PARSE-REJECTs at boot-level `RegisterPerRouteValidator` per ADR-0110 single-chokepoint. Byte-stable wording: `"lua: per-route configuration is not yet supported (lands in phase 22.3)"`. 22.3 introduces the NEW 9th canonical per-route shape per ADR-0125 §(xiv) AMENDMENT (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`) — the AMENDMENT body lands at 22.3 IMPL final Task per ADR-0125 §(xiv) anticipation paragraph (anchored at phase-22 parent SPEC commit). At 22.1 the per-route surface is uniformly rejected at config-load — REFUSES-not-defers per BRAINSTORM Q7 (deferring would require accepting per-route at parse-time + ignoring at dispatch-time, a brittle silent surface).

#### Phase 22.1 forward-pointer notes

**Phase 22.1 closes the §13-R1 (`BootRejectFixture` driver interface), §13-W (envoy-go-side `"script load error: "` wording-pinning), §11.7.5 (substring assertion firing on both sides), AMEND-10 (option-2 substring-match scope-narrow), D1 (REFUTED — arms 5 + 17 silent no-op), D3 (scenario (e) stat-counter `executions` delta IS the "Lua ran" assertion per option (a)), D5 (28th-fuzzer count CONFIRMED at 28), D7 (envoy-go headers-map is unordered `net/http.Header` + bridge `__pairs` alphabetical-snapshot RATIFIED).** No prior-phase forward-pointer was awaiting phase 22.1.

**Deferred items — 22.2 BRAINSTORM scope hand-off:**

- **Full bridge surface delta** — body methods (`:body()`, `:bodyChunks()`, `:trailers()`); metadata (`:metadata()`, `:dynamicMetadata()`); `:httpCall()` (consumer #1 of `internal/httpclient/` per phase-20 ADR-0177); crypto helpers (`:base64Decode()`, `:sha256()`); filesystem helpers (`:fileBytes()`); `:timestamp()`; `:connection()`; full `:streamInfo()`. All defer to 22.2 IMPL per parent §10. Anticipated ADRs (settled at 22.2 BRAINSTORM): ~2-4 NEW ADRs (full-bridge-API shape + httpCall dispatcher + body-buffering interaction with ADR-0128 + dynamic-metadata-bridge deferral). Likely +2 httpCall counters extend the stat surface (settled at 22.2 SPEC).
- **`*LState`-pool design (ADR-0190 escape-valve)** — D-P10 R6 STANDS WEAK-default at 22.1 IMPL (`ns/op = 69865` ~70µs/stream << 1ms threshold per parent §13-R6); ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. 22.2 may re-evaluate against the body/trailer bridge surface (which adds more bridge methods + more per-stream allocation); if the per-stream construction cost crosses the 1ms threshold there, ADR-0190 fires at 22.2 IMPL with the `*LState`-pool design.
- **AMEND-9 gopher-lua-vs-LuaJIT divergence catalogue extension** — the (a) `tostring(float)` Go shortest-round-trippable vs LuaJIT `"%.14g"`, (b) `string.format("%d", float)` Go-fmt-mismatch, (c) `pcall` error-message prefix divergences are forward-pointed by AMEND-9 for 22.2 (`lua.FormatNumber(v) string` helper recommended on `internal/lua/` for 22.2 use). 22.1's headers-bridge scope intentionally avoids these surfaces; 22.2's body/trailers bridge cannot.

**Deferred items — 22.3 BRAINSTORM scope hand-off:**

- **`Lua.SourceCodes` multi-script map activation** — arm 4 PARSE-REJECT lifts at 22.3; multi-script lookup via the `SourceCodes` map enables per-route delegation to named scripts.
- **`LuaPerRoute` 3-arm oneof override** — arm 18 PARSE-REJECT lifts at 22.3; NEW 9th canonical per-route shape per ADR-0125 §(xiv) AMENDMENT body landing at 22.3 IMPL final Task (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`).
- **Per-route 3-tier dispatch** — listener-default → SourceCodes-named-script → per-route DataSource override; settled at 22.3 SPEC.

**No new ADR-0044 escape-valve fired at phase-22.1 IMPL (D-hypothesis per BRAINSTORM Q10 WEAK HOLD STANDS):** the planner-time D-P10 R6 escape-valve gate (per parent §13-R6 RATIFIED-PENDING-IMPL) evaluated GREEN — `ns/op = 69865 ≤ 1_000_000` threshold; ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. 2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed at the 22.1 IMPL Task 16 atomic landing (ADR-0188 + ADR-0189; §Context anchored at parent SPEC commit `41ccee7` per parent SPEC §4.1 + §4.4 + ADR-0044 in-place edit discipline). ADR-0125 §(xiv) AMENDMENT-anticipation paragraph UNCHANGED at this 22.1 IMPL (anchored at parent SPEC commit; AMENDMENT body lands at 22.3 IMPL final Task).

**Fixture-0026 cross-side green-light at phase-22.1 IMPL Task 15:** the differential fixture `0026-http-lua-headers-bridge` ships at phase-22.1 phase-done as **full cross-side byte-exact for 6 wire-interactive scenarios (a)-(f)** via existing `CompareBytes` runner step 7 + **scenario (g) substring-match** via NEW OPTIONAL `BootRejectFixture` driver interface per parent §13-R1 + AMEND-10 option-2. NEW `BackendKind=HTTPLua = 22` constant + NEW `BootRejectFixture` interface + `tryStartReferenceProxy` / `tryStartSubjectProxy` variants + `runBootRejectFixture` runner branch landed at Task 13. 6-listener topology (Approach 5: one listener per scenario) selected per Task 14 — honors 22.1 SPEC §9.1's per-scenario `.lua` script separation; mirrors fixture-0018-rbac + fixture-0023-extproc multi-listener precedent. envoy-go-side `"script load error: "` wording-pinning at `cmd/envoy-go/main.go:191` + `maybeWrapLuaScriptLoadError` helper landed at Task 15 closing §13-W + AMEND-10 + §11.7.5.

#### Phase 22.2 full bridge surface delta (per ADR-0190 + ADR-0191 + ADR-0192 + IN-PLACE AMEND on ADR-0177)

Phase 22.2 ships the FULL Envoy↔Lua bridge surface delta on top of 22.1's pragmatic-middle: every upstream-parity bridge method that 22.1 deferred via Lua-runtime-error becomes a real bridge method at 22.2. **8 bridge surface families land at 22.2 phase-done** (per BRAINSTORM §1.1 + 22.2 SPEC §1 + §3.5 + ADR-0192 §Decision):

1. **Body bridge** (`:body()` whole-buffer + `:bodyChunks()` chunked iterator) — implemented as Lua coroutine yield/resume via ADR-0191's `YieldFromBridge` helper + `vm.NewThread` + `vm.Resume`. Defensive copy at endStream per §11.3 D3 RECOMMENDED — Lua owns the resulting Go string (`lua.LString(string(f.decodedBodyBytes))`) across coroutine yield/resume + HCM dispatch goroutine lifetimes. 16 MiB cap per stream via SPEC §6 arm-21 runtime-rejection (`"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`). **D-P10 R6 STANDS WEAK-default at 22.2 IMPL** (`ns/op = 98157` ~98µs/stream at the FULL 22.2 bridge surface — only 1.4× the 22.1 baseline of ~70µs; well under 1 ms threshold; conditional ADR-0193 *LState-pool gate NOT consumed at 22.2 phase-done; carries forward to 22.3 BRAINSTORM unconsumed). `BenchmarkBodyBridge_DefensiveCopy_PerStream` (Task 15) reports sub-MB body at `103268 ns/op` (~103µs; gate ≤ 1ms; 9.7× under) + 16-MiB-saturated at `9313623 ns/op` (~9.3ms; gate ≤ 100ms; 10.7× under) — D3 closure threshold gates met; option (a) defensive-copy STANDS at 22.2 phase-done.

2. **Trailers bridge** (`:trailers()` returning a headers-shaped userdata with 8 mutation methods + `__pairs` alphabetical-snapshot via 22.1's `installPairsShim`) — lazy-available (returns nil if no trailers received yet). The trailers metatable mirrors 22.1's headers metatable factory exactly per SPEC §3.5; the only operational difference is the lazy-availability discipline.

3. **Metadata bridge** (`:metadata()` callable empty userdata at v1.32.4 binding-gap per §11.6 D1 closure; NEVER nil per upstream `MetadataMapWrapper` pattern) + dynamic-metadata access via `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata(filter_name)` consuming ADR-0190's `internal/dynamicmetadata/` framework primitive at first co-consumer. **`:metadata()` returns empty-callable at 22.2** because the binding source (`Lua.SourceCodes` `filter_context` field 4 + `LuaPerRoute.filter_context` field 4) is ABSENT from v1.32.4 binding; activates at the v1.37.x binding bump phase per parent AMEND-12. **`:dynamicMetadata():get(filterName, key)` envoy-go-strict signature** — 2-arg flat accessor instead of upstream's chained `:get(filterName):get(key)` wrapper. The flat accessor is locked in by `metadata_test.go` unit tests + the production `metadata.go` IMPL; the chained-wrapper shape is intentionally NOT replicated (operationally equivalent for script authors; envoy-go-strict simplification). Documented under the "envoy-go-strict departure" notes below.

4. **Connection bridge** (`:connection():ssl()` 12-method cert/session surface consuming NEW `FilterChain.tlsConnectionState *tls.ConnectionState` field per §11.5; extends ADR-0144 TLS state plumbing pattern). Returns nil on plaintext connections; on TLS-active connections returns a userdata exposing `:peerCertificatePresented()`, `:peerCertificateValidated()`, `:uriSanLocalCertificate()`, `:sha256PeerCertificateDigest()`, `:serialNumberPeerCertificate()`, `:issuerPeerCertificate()`, `:subjectPeerCertificate()`, `:uriSanPeerCertificate()`, `:dnsSansPeerCertificate()`, `:dnsSansLocalCertificate()`, `:urlEncodedPemEncodedPeerCertificate()`, `:urlEncodedPemEncodedPeerCertificateChain()`, `:sessionId()`, `:ciphersuiteId()`, `:tlsVersion()`, `:validFromPeerCertificate()`, `:expirationPeerCertificate()`. **H1 + H2 seeding symmetric** at `internal/filter/hcm/connection.go` (H1) + `h2dispatch.go` (H2) per Task 6 — both seed `chain.SetTLSConnectionState(...)` before `RunDecodeHeaders` so the bridge sees the same state on both protocol arms.

5. **httpCall bridge** (`:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` cluster-based dispatch + sync-default + optional async flag) — consumer-#1 of phase-20's `internal/httpclient/` framework primitive at the IN-PLACE AMEND surface per ADR-0177 §Decision AMENDMENT body. **R5 RATIFIED at 22.2 Task 4**: first cross-phase co-consumer validation of phase-20's `internal/httpclient/` primitive — `Client.ClusterDispatch(ctx, clusterName, request, clusterMgr)` resolves cluster name → endpoint via `Cluster.PickEndpoint()` → rewrites `request.URL.Host` to endpoint addr → constructs temp `*http.Client` honoring cluster TLS config + retry/timeout policy. Sync arm yields the script's child coroutine via `YieldFromBridge`; the dispatch goroutine drives `parent.Resume(child, nil, responseHeaderMap, responseBody)`. Async arm per AMEND-22.2-3 D6 closure is PURE FIRE-AND-FORGET — script gets 0 return values; no yield; response/error fully discarded; matches upstream `lua_filter.cc:400-416` exactly.

6. **Crypto bridge** (6 methods in-package at `crypto.go`) — `:base64Escape` + `:base64Decode` + `:sha256` + `:sha512` + `:importPublicKey(pem)` + `:verifySignature(publicKey, data, signature, hashAlgorithm)`. `:base64Escape` is upstream-parity (Go `encoding/base64.StdEncoding` byte-identical to `absl::Base64Escape`). The other 5 are classified per D8 PLAN-time empirical scrape: **2 upstream-parity at PublicKeyWrapper userdata return scope** (`:importPublicKey` + `:verifySignature`; calling convention pinned to mimic upstream `wrappers.h:415-427`) + **3 envoy-go-strict extensions** (`:sha256` + `:sha512` + `:base64Decode` NOT in upstream v1.37.2 `StreamHandleWrapper` at any scope). Runtime-rejection arm 22 (`"lua: %s: %w"` wrapping `crypto/x509.ParsePKIXPublicKey` error) fires on malformed PEM input.

7. **Filesystem + clock bridge** (`misc.go`) — `:fileBytes(path)` unrestricted FS + 16 MiB cap (inherits 22.1 Task 11 `Filename` DataSource cap pattern via `io.LimitReader`); `:timestamp(unit?)` wall-clock via `time.Now()` with `'milliseconds'` default (also accepts `'nanoseconds'`, `'microseconds'`, `'seconds'`). **`:fileBytes` is envoy-go-strict per D8** (NOT in upstream v1.37.2 at any scope per the PLAN scrape). `:timestamp` is upstream-parity at the API surface level (operationally non-deterministic by design — REFERENCE-LESS subject-only at fixture-0027).

8. **streamInfo-full** (11-method surface; extends 22.1's 4-method subset by 7 NEW methods) — `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()` join 22.1's `:protocol()` + `:routeName()` + `:downstreamLocalAddress()` + `:downstreamDirectRemoteAddress()`. `:upstreamHost()` + `:upstreamCluster()` return empty string until upstream endpoint selection completes (encode-side; both populated AFTER `RunAction` per the chain dispatch protocol — scripts using these in `envoy_on_request` see empty strings, which is operationally equivalent to upstream). `:downstreamSslConnection()` returns the same userdata shape as `:connection():ssl()` (alias for operator convenience matching upstream `wrappers.cc::StreamInfoWrapper::luaDownstreamSslConnection`).

**`:filterState()` IN-PACKAGE per Q9 + AMEND-22.2-4** — string-keyed `map[string]any` per-stream `filterState *filterStateBucket` field on `*filter` struct (per §3.5 + AMEND-22.2-4 + §11.8 D4 closure). 2 envoy-go-strict divergences from upstream Envoy v1.37.2:

- `:filterState():set(name, value)` mutation surface EXPOSED (upstream `FilterStateWrapper` is strictly read-only — `:get` only — because C++ filters mutate FilterState directly via `streamInfo().filterState().setData(...)`; envoy-go has no Go-side mutation analog at 22.2; the `:set` exposure is the envoy-go-strict equivalent).
- `:filterState():get(name)` typed Lua-value marshaling at return (upstream always returns `serializeAsString()` strings; envoy-go converts via `gopher-lua.LValue` to native Lua types — string stays string, int converts to `lua.LNumber`, bool to `lua.LBool`, table-typed values to `lua.LTable`).

**`internal/filterstate/` framework primitive extraction stays DEFERRED at 22.2** — per Q9 EXTRACT-NOW-only-when-trigger-fires + BRAINSTORM §1.6. The filter-state primitive lives IN-PACKAGE at `internal/filter/http/lua/filterstate.go` at 22.2. The second consumer of cross-filter filter-state (future cross-filter passing surface) triggers the `internal/filterstate/` extraction at that future phase.

**`internal/dynamicmetadata/` cross-phase deferral-lift discipline (per ADR-0190 §Consequences).** Phase 22.2 lands the NEW `internal/dynamicmetadata/` framework primitive at FIRST co-consumer (this lua bridge's `:dynamicMetadata` + `:dynamicTypedMetadata` access). Phases 16 (rbac) / 17 (jwt_authn) / 18 (ext_authz) / 19 (ext_proc) / 20 (oauth2) each deferred dynamic-metadata access by their respective filters with BEHAVIOR_CONTRACT.md "operator-visibility deferred to future" notes. **Those deferral notes carry forward AS-IS at this 22.2 phase-done** — each prior-phase BEHAVIOR_CONTRACT note converts from "deferred" to "lifted via `internal/dynamicmetadata`" at the lift-phase's next-touchpoint (the future phase that next edits the prior-phase filter's deferral note). The lift discipline is documented in this paragraph + ADR-0190 §Consequences cross-phase deferral-lift expectation paragraph.

**Production HCM coroutine orchestration at Task 19a.** `internal/filter/http/lua/decode_headers.go` + `encode_headers.go` invoke `envoy_on_request` / `envoy_on_response` via `vm.NewThread()` + `vm.Resume(child, fn, ud)` per §11.1 D2 closure. On `ResumeYield`: (a) sync httpCall yield branch closes `httpCallReady` + waits on `httpCallDone` so the dispatch goroutine drives Resume to script completion synchronously inside DecodeHeaders before the chain returns; (b) body-yield branch returns `Continue` so `RunDecodeData` / `RunEncodeData` fires and resumes the coroutine via `accumulateRequestBody` / `accumulateResponseBody` at endStream. **"Continue-on-body-yield" trade-off documented**: returning Continue (vs StopIteration) is required because envoy-go's HCM serializes Headers→Data — returning StopIteration would deadlock since the HCM's body-read loop never starts. The trade-off: subsequent decode-side filters see request headers BEFORE Lua's post-:body() mutations; for the fixture-0027 lua→router topology this is benign (router runs RunAction AFTER RunDecodeData completes, by which time the script has resumed + mutated headers). **Multi-decoder-filter topologies that depend on body-after-yield header-mutation visibility ARE a known limitation** flagged for REVIEW.md / phase-22.3 / a future framework phase to introduce a "park-headers-iteration-pending-body" cooperative discipline.

**3 NEW runtime-rejection arms 20-22 (per SPEC §6 + W2 byte-stable wording PINNED at Task 14):**

| Arm | Trigger | Byte-stable wording |
|---|---|---|
| 20 | `:httpCall("", ...)` empty cluster name | `"lua: httpCall: cluster name must not be empty"` |
| 21 | `:body()` accumulated bytes > 16 MiB cap | `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"` (cap value = 16777216) |
| 22 | `:importPublicKey(pem)` malformed PEM input | `"lua: %s: %w"` wrapping `crypto/x509.ParsePKIXPublicKey` error |

These are RUNTIME-REJECTs (Lua-runtime-error via `luaL_error`), NOT PARSE-REJECTs at config-load time. The 19-arm config-load PARSE-REJECT roster from 22.1 remains UNCHANGED at 22.2 config-load. All 3 wordings byte-pinned via the package's `compiled_config_test.go::TestRuntimeRejectConstants_ByteExactWording` + `httpcall_test.go` / `body_test.go` / `crypto_test.go` assertions.

#### envoy-go-strict departures introduced by phase 22.2

The 5 NEW envoy-go-strict counter rows in the stat table above each codify an envoy-go-strict departure record. In addition, 2 NEW `:filterState()` divergence records + 4 NEW D8-classified crypto/fileBytes divergence records land at 22.2 IMPL atomic landing (per BEHAVIOR_CONTRACT 15-edit bundle + 22.2 SPEC §14):

**envoy-go-strict departure — `:filterState():set(name, value)` mutation surface (per AMEND-22.2-4 + §11.8 D4 closure).** Upstream Envoy v1.37.2 `FilterStateWrapper` (`source/extensions/filters/common/lua/wrappers.h`) exposes ONLY `:get(name)` — strictly read-only surface. envoy-go's `internal/filter/http/lua/filterstate.go::FilterStateBucket::filterStateSet` EXPOSES `:set(name, value)` accepting any Lua value (string, number, boolean, table) + stores at the per-stream `map[string]any` field. Rationale: upstream's read-only posture is because C++ filters mutate FilterState directly via the typed `streamInfo().filterState().setData(...)` C++ API — there is no Go-side analog at 22.2 (cross-filter mutation lives via `internal/dynamicmetadata/` for now). Exposing `:set` from Lua is the envoy-go-strict equivalent of the upstream C++-side mutation surface. Departure is OPERATIONALLY-VISIBLE only to operator scripts that try to mutate filter-state from Lua under upstream Envoy v1.37.2 (which raises `attempt to call method 'set' (a nil value)`); envoy-go silently accepts the mutation + persists at the per-stream bucket. Documented for operators porting upstream-Envoy Lua scripts to envoy-go.

**envoy-go-strict departure — `:filterState():get(name)` typed Lua-value marshaling at return (per AMEND-22.2-4 + §11.8 D4 closure).** Upstream Envoy v1.37.2 `FilterStateWrapper::luaGet` (`wrappers.cc::FilterStateWrapper::luaGet`) ALWAYS returns `lua_pushlstring(state, ...)` with the bytes from the FilterState object's `serializeAsString()` method — strings only, never typed Lua values. envoy-go's `internal/filter/http/lua/filterstate.go::FilterStateBucket::filterStateGet` returns native Lua values per `gopher-lua.LValue` conversion: stored Go `string` → `lua.LString`; stored Go `int` / `int64` / `float64` → `lua.LNumber`; stored Go `bool` → `lua.LBool`; stored `map[string]any` → `lua.LTable` (recursive conversion). Departure rationale: envoy-go's Go-side type system allows typed storage (the `map[string]any` bucket can hold any Go interface value); silently downcasting to string at `:get` would lose information operators may need. Operator-visible only via scripts that do `type(filter_state:get(k))` (envoy-go returns native types; upstream always returns `"string"`).

**envoy-go-strict departure — `:dynamicMetadata():get(filter_name, key)` 2-arg flat accessor (per Task 9 IMPL + ADR-0192 §Decision body).** Upstream Envoy v1.37.2 `DynamicMetadataMapWrapper::luaGet` (`source/extensions/filters/common/lua/wrappers.cc`) returns a CHAINED wrapper at `:get(filterName)` — the script then calls `:get(key)` on the returned wrapper to fetch a specific key under that filter (per `:dynamicMetadata():get("envoy.filters.http.jwt_authn"):get("issuer")`). envoy-go's `internal/filter/http/lua/metadata.go::dynamicMetadataGet` is a 2-arg flat accessor — the script calls `:get(filterName, key)` directly (per `:dynamicMetadata():get("envoy.filters.http.jwt_authn", "issuer")`). Departure rationale: the flat accessor is mechanically simpler (no chained wrapper userdata allocation; no per-`:get` metatable install) at minor cost to upstream-script-portability. Operator-visible at script-port time only. Documented at the `metadata_test.go::TestDynamicMetadata_GetFlatSignature` byte-stable signature test.

**envoy-go-strict departure — `:body()` return shape (Go string vs upstream Buffer userdata) (per Task 7 + Task 19a IMPL + ADR-0192 §Decision body).** Upstream Envoy v1.37.2 `StreamHandleWrapper::luaBody` returns a `Buffer` userdata wrapping the accumulated body bytes; scripts call `:length()` + `:getBytes(start, end)` on the wrapper to access bytes selectively. envoy-go's `body.go::filterBody` returns a Lua string via `lua.LString(string(f.decodedBodyBytes))` directly — scripts use `#body` / `string.sub(body, ...)` / `string.byte(body, ...)` against the Go-string-backed Lua string. Departure rationale: per §11.3 D3 + §11.2.1 evidence, the Lua-string shape is byte-fidelity-equivalent (Go strings are immutable length-prefixed; NUL-safe; multi-MB tolerant); the defensive-copy posture per §11.3 D3 is mechanically simpler than the wrapped Buffer + length tracking. Operator-visible at script-port time only.

##### envoy-go-strict departure — `httpcall_total` counter

**Departure type:** envoy-go-strict counter ADDITION to the `errors` + `executions` upstream baseline.

Upstream Envoy v1.37.2 `lua_filter.cc` defines `ALL_LUA_FILTER_STATS(COUNTER)` macro containing ONLY `errors` + `executions` (per parent SPEC §11.5.3 + 22.2 SPEC §7.1). The `httpcall_total` counter — increments per `:httpCall()` invocation across both sync + async dispatch arms — has no upstream analog. envoy-go emits this counter to give operators visibility into outbound-call rate per filter / stat_prefix (operator scripts that dispatch authentication-side-channel / geo-IP / abuse-detection lookups via `:httpCall` are observability surfaces operators actively want). Collision-free verified at 22.2 SPEC §11.7 against the upstream macro (only `errors` + `executions`). Operator-visibility rationale matches the `respond_calls` precedent at 22.1.

##### envoy-go-strict departure — `httpcall_failures` SYNC-ONLY counter

**Departure type:** envoy-go-strict counter ADDITION + SYNC-ONLY scoping per AMEND-22.2-3 D6 closure.

Upstream Envoy v1.37.2 has no per-script-httpCall failure counter. envoy-go emits `httpcall_failures` to give operators visibility into outbound-call failure rate (status-driven retry exhaustion via `httpclient.Options.RetryPolicy.RetryOnStatus`; transport errors via `httpclient.Client.ClusterDispatch` non-nil error). **SYNC-ONLY scoping** per AMEND-22.2-3 D6 closure: upstream's `asynchronous=true` arm wires failures to `noopCallbacks` global singleton (response fully discarded; failures invisible to filter-stats per upstream-parity at the OBSERVABILITY layer). envoy-go preserves this SYNC-ONLY upstream-parity property — async dispatch failures are invisible at filter-stats. Operators that need failure visibility must use sync `:httpCall` dispatch (omit `asynchronous=true` or set it explicitly false). Documented at this departure record + the `httpcall_test.go::TestHTTPCall_FailureCounterSyncOnly` byte-stable assertion.

##### envoy-go-strict departure — `httpcall_timeouts` SYNC-ONLY counter

**Departure type:** envoy-go-strict counter ADDITION + SYNC-ONLY scoping per AMEND-22.2-3 D6 closure.

Same shape as `httpcall_failures`: envoy-go-strict counter for sync-only timeout firings (`context.DeadlineExceeded` from the retry-loop budget exhaustion); async dispatch timeouts invisible per upstream-parity discipline.

##### envoy-go-strict departure — `body_buffered_bytes_total` counter

**Departure type:** envoy-go-strict counter ADDITION.

Upstream Envoy v1.37.2 has no per-script body-buffer accumulation counter. envoy-go emits `body_buffered_bytes_total` to give operators visibility into cumulative body bytes accumulated across all streams that invoke `:body()` or `:bodyChunks()` (capacity-planning signal + early-warning for operator scripts that buffer multi-MB bodies in steady state — operationally important because the 16 MiB-cap per stream limits per-stream blast-radius but doesn't limit cumulative buffering). Increment site: the defensive-copy moment at endStream per §11.3 D3 (one increment per accumulated stream-final byte).

##### envoy-go-strict departure — `coroutine_yields_total` counter

**Departure type:** envoy-go-strict counter ADDITION.

Upstream Envoy v1.37.2 has no per-script coroutine-yield counter. envoy-go emits `coroutine_yields_total` to give operators visibility into yield-heavy scripts (inefficient body-streaming patterns; sync `:httpCall`-heavy patterns). Increment site: each `YieldFromBridge(L, ...)` call (one per `:body()` pre-endStream invocation; one per sync `:httpCall` dispatch; one per `:bodyChunks()` next-chunk-not-yet-buffered call). Cumulative counter; envoy-go-strict naming convention parallels other `_total` cumulative counters in the project.

##### envoy-go-strict departure — `:sha256` crypto bridge method (per D8 PLAN-time scrape — 22.2 SPEC §13-R7)

**Departure type:** envoy-go-strict bridge method ADDITION at script scope (no upstream analog at any wrapper).

Upstream Envoy v1.37.2's `StreamHandleWrapper` (`source/extensions/filters/http/lua/lua_filter.h:226-357` per `DECLARE_LUA_FUNCTION` grep) does NOT expose `:sha256`. PARTIAL-PLAN-time-scrape confirmed against `source/extensions/filters/common/lua/wrappers.h` `PublicKeyWrapper` + `CryptoUtility` + script-global helpers: NO upstream surface at any scope. envoy-go's `internal/filter/http/lua/crypto.go::filterSha256` consumes `crypto/sha256.Sum256(input)` + returns the 32-byte digest as a Lua string. Calling convention: `request_handle:sha256(bytes) → string` (returns raw bytes, NOT hex-encoded — operator scripts that want hex output should chain via `:sha256(bytes):base64Escape()` or similar). Documented at this departure record + the `crypto_test.go::TestCrypto_Sha256_Wiring` byte-stable assertion.

##### envoy-go-strict departure — `:sha512` crypto bridge method (per D8 PLAN-time scrape — 22.2 SPEC §13-R7)

**Departure type:** envoy-go-strict bridge method ADDITION at script scope.

Same shape as `:sha256` — no upstream analog at any wrapper scope per D8 PLAN-time scrape. envoy-go's `internal/filter/http/lua/crypto.go::filterSha512` consumes `crypto/sha512.Sum512(input)` + returns the 64-byte digest as a Lua string. Calling convention: `request_handle:sha512(bytes) → string`. Documented at this departure record + the `crypto_test.go::TestCrypto_Sha512_Wiring` byte-stable assertion.

##### envoy-go-strict departure — `:base64Decode` crypto bridge method (per D8 PLAN-time scrape — 22.2 SPEC §13-R7)

**Departure type:** envoy-go-strict bridge method ADDITION at script scope.

Upstream Envoy v1.37.2's `StreamHandleWrapper` exposes ONLY `:base64Escape` (encode direction; `lua_filter.cc:204` → `static_luaBase64Escape` → `absl::Base64Escape`). The decode direction `:base64Decode` is NOT in upstream at any scope per D8 PLAN-time scrape. envoy-go's `internal/filter/http/lua/crypto.go::filterBase64Decode` consumes Go `encoding/base64.StdEncoding.DecodeString(input)` + returns the decoded bytes as a Lua string, or raises `luaL_error` with byte-stable wording `"lua: base64Decode: %w"` wrapping the `*base64.CorruptInputError` from the stdlib if the input is malformed. Calling convention: `request_handle:base64Decode(str) → string`. Documented at this departure record + the `crypto_test.go::TestCrypto_Base64Decode_Wiring` byte-stable assertion.

##### envoy-go-strict departure — `:fileBytes` filesystem bridge method (per D8 PLAN-time scrape — 22.2 SPEC §13-R8)

**Departure type:** envoy-go-strict bridge method ADDITION at script scope.

Upstream Envoy v1.37.2 has no `:fileBytes` (or any `fileBytes` / `luaFile*` method anywhere in the Lua filter source per D8 PLAN-time `grep` scrape across `lua_filter.cc`, `wrappers.cc`, `lua.h`, `wrappers.h`). envoy-go's `internal/filter/http/lua/misc.go::filterFileBytes` consumes `os.Open` + `io.LimitReader(f, maxFileBytesScriptBytes+1)` (16 MiB cap inheriting the 22.1 Task 11 `Filename` DataSource cap pattern) + returns the file contents as a Lua string. PARSE-REJECTs at: name-empty (`"lua: fileBytes: path empty"`); `os.Open` failure (wrap); read exceeds 16 MiB cap (`"lua: fileBytes: file %q exceeds the maximum read size of %d bytes"`). Calling convention: `request_handle:fileBytes(path) → string` (raises `luaL_error` on any of the failure arms). Documented at this departure record + the `misc_test.go::TestMisc_FileBytes_Wiring` byte-stable assertion.

##### D8 disposition (per 22.2 PLAN-time empirical upstream-Envoy-v1.37.2 scrape — closes 22.2 SPEC §13-R7 + §13-R8 + AMEND-22.2-2)

The 22.2 PLAN session ran a targeted upstream-Envoy-v1.37.2 WebFetch scrape against `source/extensions/filters/http/lua/{lua_filter.h,lua_filter.cc}` + `source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` per SPEC §13-R7 + §13-R8 + AMEND-22.2-2 PARTIAL-REFUTATION. Classification outcome:

- **2/6 upstream-parity at the PublicKeyWrapper userdata return scope:** `:importPublicKey(pem) → PublicKeyWrapper` (`lua_filter.h:315`) + `:verifySignature(publicKey, data, signature, hashAlgorithm)` on `StreamHandleWrapper` (`lua_filter.h:303`). Calling convention pinned to mimic upstream's PublicKeyWrapper userdata return scope per `wrappers.h:415-427` (anti-departure for the calling convention; the implementation is envoy-go-Go-native but the script-facing surface is upstream-equivalent).
- **4/6 envoy-go-strict** (NOT in upstream v1.37.2 at any scope per D8 PLAN scrape): `:sha256` + `:sha512` + `:base64Decode` + `:fileBytes`. Each anchored as a departure record above.

The 5-record BEHAVIOR_CONTRACT bundle anticipated at 22.2 SPEC §14 (5 counters + 2 filterState) scales to **11 envoy-go-strict departure records at 22.2 phase-done** (5 counters + 2 filterState + 1 metadata flat-accessor + 1 body return-shape + 4 D8 crypto/fileBytes = 13 records under the broader documented-divergence count; the SPEC §14 "11 envoy-go-strict departure records" formulation collapses the 2 minor surface-shape records under the 7-bridge-surface umbrella). Fixture-0027 scenario (h) `:fileBytes` reclassified from cross-side cross-side `CompareBytes` to REFERENCE-LESS subject-only per D8 (reference Envoy cannot run `:fileBytes` script — would error at runtime per absent-binding). Cross-references: 22.2 SPEC §13-R7 + §13-R8; AMEND-22.2-2 PARTIAL-REFUTATION; D8 PLAN-time closure record at 22.2 PLAN §"Planner-time deferred-decision resolution" D8 paragraph.

#### Phase 22.2 forward-pointer notes

**Phase 22.2 closes the §13-R5 (RATIFIED at Task 4 — first co-consumer validation of phase-20's `internal/httpclient/` primitive at the `:httpCall()` bridge via IN-PLACE AMEND on ADR-0177), §13-R7 (D8 PLAN-scrape — 2/6 upstream-parity + 4/6 envoy-go-strict), §13-R8 (D8 PLAN-scrape — `:fileBytes` envoy-go-strict), §13-R10 (30 fuzzer count CONFIRMED at Task 16 — `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`), §13-R11 (REUSE existing `runReferenceLessFixture` driver helper — no new `RunSubjectOnlyHTTPLua`), §13-W2 (3 runtime-reject arms 20-22 byte-stable wording PINNED at Task 14), D1 (CLOSED at SPEC §11.6 — `:metadata()` callable empty userdata never-nil), D2 (CLOSED at SPEC §11.1 — gopher-lua native `LState.NewThread/Yield/Resume`), D3 (CLOSED at PLAN — option (a) defensive copy at endStream + Task 15 perf-benchmark threshold gates met), D4 (CLOSED at SPEC §11.8 — string-keyed `map[string]any` filter-state + 2 envoy-go-strict divergences), D5 (CLOSED at PLAN — option (f-B) cert-fingerprint-only cross-side via `:sha256PeerCertificateDigest()` + Task 17 cert fixture plumbing), D6 (CLOSED at SPEC §11.7 — `:httpCall` async = pure fire-and-forget), D7 (CLOSED at SPEC §11.9 — 30-fuzzer post-22.2 count), D8 (CLOSED at PLAN via empirical upstream-Envoy-v1.37.2 WebFetch scrape).** §13-R6 *LState-pool gate STANDS WEAK-default at 22.2 IMPL Task 15 (`ns/op = 98157` ~98µs at the FULL 22.2 bridge surface; well under 1ms threshold; ADR-0193 NOT consumed); §13-R9 body-buffer-seam STAYS embedded in ADR-0192 (Task 7 + Task 15 evaluation: yield/resume orchestration mechanically simple + defensive-copy discipline one line per call site + sub-MB body at `103268 ns/op` ~103µs + 16-MiB-saturated at `9313623 ns/op` ~9.3ms BOTH under D3 closure threshold gates; no ADR-warranting complexity beyond ADR-0192 §Context). **Conditional ADR-0193 NOT consumed at 22.2 phase-done**; carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot per SPEC §13-R6 + §13-R9 + §1.2 hypothesis (a).

**Deferred items — 22.3 BRAINSTORM scope hand-off (LANDED at phase-22.3 IMPL — see `#### Phase 22.3 multi-script + per-route surface delta` below):**

- **`Lua.SourceCodes` multi-script map activation** — arm 4 PARSE-REJECT LIFTED at 22.3 IMPL Task 1; the `SourceCodes` map is consumed into a `name → *Chunk` content-hash registry; multi-script lookup enables per-route delegation to named scripts. The `source_codes`-deferred reject is RETIRED; the sole `SourceCodes`-key arm is `source-codes-key-empty` (`"lua: source_codes: key must be non-empty"`).
- **`LuaPerRoute` 3-arm oneof override** — arm 18 PARSE-REJECT LIFTED at 22.3 IMPL Task 2; the real 3-arm validator (`parsePerRouteLua`) replaces the one-liner. NEW 9th canonical per-route shape LANDED as ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL Task 6 (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`).
- **Per-route 3-tier dispatch** — LANDED at 22.3 IMPL Task 3: per-route override (`disabled`→skip / `name`→registry lookup / `source_code`→memo override) → listener-default → no-op. The encode-guard changed from `f.cc.chunk==nil` to `f.vm==nil` so a per-route override on a default-less listener still fires `envoy_on_response`.
- **`*LState`-pool design (ADR-0193 escape-valve)** — D-P10 R6 STANDS WEAK-default at 22.2 IMPL Task 15 (`ns/op = 98157`). **At 22.3 IMPL Task 4 the R6 gate STANDS WEAK-default again** — `BenchmarkPerStream_PerRoute_Resolution` resolution-only `10.46 ns/op` (0 allocs) + per-stream `31.47 ns/op` (both ~5 orders of magnitude under the 1 ms gate; per-route resolution is an O(1) content-hash + memo lookup, not a new per-stream VM construction). **Conditional ADR-0194 NOT consumed; next-free ADR STAYS ADR-0194.**
- **Multi-decoder-filter "park-headers-iteration-pending-body" cooperative discipline** — the Task 19a "Continue on body-yield" trade-off remains a known limitation for multi-decoder-filter topologies; 22.3's per-route discipline does NOT surface it (per-route applies to a single lua filter instance at a time). STAYS DEFERRED to a future framework phase (see `#### Phase 22.3 forward-pointer notes` below).

**Deferred items — future cross-phase dynamic-metadata lift consumers:**

- **Phases 16 (rbac) / 17 (jwt_authn) / 18 (ext_authz) / 19 (ext_proc) / 20 (oauth2) dynamic-metadata deferral-lift** — each prior-phase BEHAVIOR_CONTRACT.md "operator-visibility deferred to future" note converts from "deferred" to "lifted via `internal/dynamicmetadata`" at the lift-phase's next-touchpoint (the future phase that next edits the prior-phase filter's deferral note). NO automatic mass-update at 22.2 phase-done — the deferral notes carry forward AS-IS until each is touched again.

**Deferred items — future `internal/filterstate/` framework primitive extraction:**

- The 22.2 filter-state surface lives IN-PACKAGE at `internal/filter/http/lua/filterstate.go` per Q9 + AMEND-22.2-4. The SECOND consumer of cross-filter filter-state (future cross-filter-passing surface for jwt_authn / ext_authz / rbac / ext_proc / oauth2 / etc.) triggers the `internal/filterstate/` extraction at that future phase. The extraction will preserve the `map[string]any` shape + the typed Lua-value marshaling discipline + the `:set` exposure (envoy-go-strict per AMEND-22.2-4).

**No new ADR-0044 escape-valve fired at phase-22.2 IMPL (D-hypothesis per BRAINSTORM Q13 WEAK HOLD STANDS):** the planner-time §13-R6 + §13-R9 conditional ADR-0193 escape-valve gates BOTH evaluated GREEN (R6 STANDS WEAK-default at `ns/op = 98157`; R9 STAYS embedded in ADR-0192 from Task 7 + Task 15). ADR-0193 NOT consumed; carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot. 3 NEW ADR §Decision + §Consequences bodies landed at the 22.2 IMPL Task 19 atomic landing (ADR-0190 + ADR-0191 + ADR-0192; §Context anchored at predecessor 22.2 SPEC commit `0d6463e` per ADR-0044 in-place edit discipline). 1 IN-PLACE AMENDMENT body landed on ADR-0177 §Decision (Client.ClusterDispatch + FactoryCtx.ClusterManager + Cluster.UpstreamTLSConfig — no new ADR number consumed per ADR-0044 in-place edit discipline; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). ADR-0125 §(xiv) AMENDMENT-anticipation paragraph UNCHANGED at this 22.2 IMPL (anchored at parent SPEC commit; AMENDMENT body lands at 22.3 IMPL final Task).

**Fixture-0027 mixed-mode green-light at phase-22.2 IMPL Task 18 + 19a:** the differential fixture `0027-http-lua-full-bridge` ships at phase-22.2 phase-done as **8 deterministic cross-side scenarios** (a/b/c/d/e/f-cert-fingerprint/g/i — body / bodyChunks / trailers / metadata-empty / dynmd / connection-SSL-fingerprint / crypto-base64Escape / streamInfo-upstream) + **5 non-deterministic REFERENCE-LESS subject-only scenarios** (h/j/k/l/m — fileBytes envoy-go-strict / httpCall sync / httpCall async / timestamp / filterState). 13 scenarios total in one fixture directory. Driver helper REUSED per §13-R11 D-P11 closure — no new `RunSubjectOnlyHTTPLua` infrastructure; the existing `runReferenceLessFixture` pattern serves both deterministic + non-deterministic cross-side cases. Cross-side discipline per §11.5 D5 closure: scenario (f) `:connection():ssl()` cross-side via `:sha256PeerCertificateDigest()` byte-exact hex digest only (cert-fingerprint-only per option (f-B)); other 11 SSL methods exercised in REFERENCE-LESS subject-only scope (subject-side wire-output well-formed). NEW cert fixture plumbing at Task 17 — minimal self-signed cert generated via openssl + plumbed through both reference + subject sides. 28 → 29 fixture directories at 22.2 phase-done.

**Production HCM coroutine orchestration green-light at phase-22.2 IMPL Task 19a (PRE-ATOMIC-LANDING):** `internal/filter/http/lua/{decode_headers.go,encode_headers.go,lua.go}` rewritten to invoke operator hooks as coroutines via `vm.NewThread()` + `vm.Resume(child, fn, ud)`. Cancellation discipline: `*filter` carries `decodeChild + decodeChildCancel + encodeChild + encodeChildCancel` fields; `OnDestroy` invokes both cancel funcs + nils the child references. ResumeYield branch dispatch: (a) sync httpCall yield → close + wait gate; (b) body-yield → Continue + RunDecodeData/RunEncodeData fires the resume at endStream. "Continue on body-yield" trade-off documented above + flagged in REVIEW.md. The orchestration design is ratified into ADR-0192 §Decision body at this 22.2 IMPL atomic landing (Task 19) per ADR-0044 in-place edit discipline.

#### Phase 22.3 multi-script + per-route surface delta (per ADR-0193 + ADR-0125 §(xiv) AMENDMENT)

Phase 22.3 lands the multi-script `Lua.SourceCodes` registry + the `LuaPerRoute` 3-arm per-route override + the NEW 9th canonical per-route shape on top of the 22.1 + 22.2 surfaces. By 22.3 phase-done every v1.32.4 `Lua` + `LuaPerRoute` proto field is CONSUMED (modulo the two v1.37.2 binding-gap fields `Lua.clear_route_cache` + `LuaPerRoute.filter_context`, ABSENT in v1.32.4 per 22.3 SPEC §11.1). It is a CONSUME + DISPATCH delta: **0 new framework primitives, 0 net-new stats, 0 net-new bridge methods, 0 net-new envoy-go-strict departure records** (all 22.3 dispositions are upstream-parity). 4 surface families land:

1. **`Lua.SourceCodes` multi-script registry active** — listener-level named scripts compiled at config-load (sorted-key iteration; resolved via the 22.1 4-arm DataSource resolution + compiled into the SHARED per-listener content-hash `CompileCache`; identical-content named scripts dedup to one cached `*Chunk`). Named scripts are dispatch TARGETS only — a named script does NOT run by itself; a per-route `name` arm references it. `default_source_code` remains the sole listener-level default. Upstream-parity (per 22.3 SPEC §11.2 Q1). The only `SourceCodes`-key config-load arm is `source-codes-key-empty` (`"lua: source_codes: key must be non-empty"`; envoy-go-strict-as-defensive-mirror — NOT a departure record).

2. **`LuaPerRoute` 3-arm per-route override active** — `disabled` (PGV `const: true`) / `name` (PGV `min_len: 1`; string-reference into `Lua.SourceCodes`) / `source_code` (`*core.DataSource`; wholesale-override). The NEW 9th canonical per-route shape (ADR-0125 §(xiv) AMENDMENT). 6 net-new config-load PARSE-REJECT arm-groups (per D-P3): `source-codes-key-empty` + `source-codes-value-gauntlet` (`"lua: source_codes[%q]: %w"`) + `per-route-override-oneof-required` (`"lua: per-route: override oneof is required"`) + `per-route-disabled-must-be-true` (`"lua: per-route: disabled must be true (PGV const:true violation)"`) + `per-route-name-min-1-rune` (`"lua: per-route: name length must be at least 1 rune"`) + `per-route-source_code-gauntlet` (`"lua: per-route: source_code: %w"`). **Arm 3 (reserved-name) + arm 7 (dangling-name) are NOT present** (dropped per the two upstream-parity NOTES below).

3. **Per-route 3-tier dispatch** — per-route override (`disabled` → both hooks skipped, no VM built; `name` → registry lookup; `source_code` → memo-resolved override `*Chunk`) → listener `DefaultSourceCode` → no-op. Matches upstream `getPerLuaCodeSetup()` precedence; the per-stream binding is an O(1) content-hash + proto-pointer-memo lookup (no new per-stream VM construction). The encode-guard gates on `f.vm == nil` (the per-stream decode-side "a script ran" sentinel) so a per-route override on a default-less listener still fires `envoy_on_response`.

4. **Per-route `disabled: true` skips both hooks** — upstream-parity (per 22.3 SPEC §11.2 Q4); the filter is a no-op for the route (no VM built; no chain omission).

**Upstream-parity NOTE — dangling per-route `name` = silent no-op (per AMEND-22.3-1; ADR-0193).** A per-route `name` referencing a key ABSENT from the listener-level `Lua.SourceCodes` map runs NO script for that route — there is NO config-load PARSE-REJECT and NO runtime error. This mirrors upstream Envoy v1.37.2 (`FilterConfig::perLuaCodeSetup(name)` returns `nullptr` on a map miss → `function_ref = LUA_REFNIL` → neither hook runs). **envoy-go does NOT fail-fast on a dangling per-route `name`** — distinguishing it from the jwt_authn LISTENER-level fail-fast (a dangling jwt_authn `requirement_name` is parse-rejected at boot per ADR-0153). The lua disposition matches jwt_authn's PER-ROUTE half (runtime-resolve, per ADR-0153 §1.1 amendment 6), except lua silently no-ops where jwt_authn emits 403 (lua is not an authorization filter). This is upstream-parity, NOT an envoy-go-strict divergence — **0 departure records.**

**Upstream-parity NOTE — no reserved-name discipline (per BRAINSTORM decision #2; ADR-0193).** `Lua.source_codes` keys are free-form; `Lua.default_source_code` is an independent proto field (no PGV map-key rule; no name addresses the default). A `SourceCodes` key cannot "collide" with the default. Upstream-parity, NOT an envoy-go-strict divergence — **0 departure records.**

**Stat surface UNCHANGED at 107 (SHARED-vacuous per the 9th canonical; per ADR-0125 §(xiv) + ADR-0154).** Per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; `LuaPerRoute` has no `stat_prefix` field. 0 net-new counters; no stat-table edit.

**ADR-0125 §(xiv) cross-reference.** The 9th canonical per-route shape (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource`-typed `wholesale-override`) + the SHARED stat-discipline are codified at DECISIONS.md ADR-0125 §(xiv) IN-PLACE AMENDMENT body (canonical roster grows 8 → 9), landed at the same 22.3 IMPL Task 6 atomic landing as this subsection per ADR-0044 in-place edit discipline. The combined package-shape decision lives at ADR-0193 §Decision + §Consequences.

**Differential coverage (29 → 31 fixture directories; the authorized two-directory amendment).** `0028-http-lua-multi-script-and-per-route` (CROSS-SIDE multi-listener; 5 per-route selection scenarios (a) listener-default / (b) per-route `name`→named / (c) per-route `source_code`-override / (d) per-route `disabled`-skip / (e) distinct-named-key selection + (b2) dangling-name silent no-op — all byte-exact `CompareBytes`) + `0029-http-lua-source-codes-boot-reject` (BOOT-REJECT; `source_codes{bad}` compile-error; common stderr substring `near '-'`). The framework's `runFixture` dispatches one branch per directory (cross-side XOR boot-reject), so the SPEC §8's single-fixture "5 cross-side + 3 boot-reject (29→30)" is realized as the AUTHORIZED two-directory amendment 29→31. The subject-only (f) `source_codes` key-empty + (h) per-route `source_code`-DataSource-failure arms are covered at the unit layer (Task 1 + Task 2). **0 envoy-go-strict departure records at 22.3 — the `### envoy.filters.http.lua` departure-record count STAYS 14** (3 from 22.1 + 11 from 22.2).

#### Phase 22.3 forward-pointer notes (parent-row-22 closure)

**Phase 22.3 closes the §13-R6 *LState-pool gate (STANDS WEAK-default at `ns/op = 10.46`/`31.47` resolution-only/per-stream — conditional ADR-0194 NOT consumed; next-free ADR STAYS ADR-0194), D-P1 (option (b') compile-with-cache-hit at bind + proto-pointer memo + the no-re-read realization), D-P2 (two-directory fixture split — owner-authorized 29→31), D-P3 (6 net-new config-load arm-groups; arms 3 + 7 dropped), AMEND-22.3-1 (dangling-name silent no-op), and the parent-row-22 closure.** Phase 22.3 is the FINAL sub-phase of parent phase 22; ROADMAP row 22.3 + parent row 22 both flip `in-progress → done` at 22.3 IMPL phase-done. The §9 HTTP-filters family closes from 4 remaining rows (post-phase-21) to 3 remaining: `wasm`, `admission_control`, `global rate limit`.

**Deferred items — future cross-phase / binding-bump:**

- **`:metadata()` per-route source activation + `LuaPerRoute.filter_context` (v1.37.2 field 4)** — ABSENT in the v1.32.4 binding (per parent AMEND-12). 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` per-route data; `:metadata()` continues to return the 22.2 callable empty userdata. Activates at the v1.37.x binding-bump phase.
- **`Lua.clear_route_cache` (v1.37.2 field 5)** — ABSENT in v1.32.4 (per parent AMEND-12); future binding-bump phase.
- **`internal/lua/` consumers #2/3/4 (cluster-specifier / access-logger / string-matcher Lua)** — ADR-0188's EXPLICIT API-REVISION ALLOWANCE STAYS scoped to consumer-#2 (UNCHANGED at 22.3). Each future consumer phase BRAINSTORM revisits the `internal/lua/` API shape.
- **Multi-decoder-filter "park-headers-iteration-pending-body" cooperative discipline** — the 22.2 "Continue on body-yield" trade-off (22.2 REVIEW §6.1) STAYS deferred to a future framework phase; 22.3's per-route discipline does NOT surface it (per-route applies to a single lua filter instance at a time).
- **`internal/filterstate/` framework primitive extraction** — `:filterState()` STAYS IN-PACKAGE (landed at 22.2); a future second filter-state consumer triggers the extraction.

**No new ADR-0044 escape-valve fired at phase-22.3 IMPL (D-hypothesis WEAK HOLD STANDS):** the §13-R6 *LState-pool conditional ADR-0194 gate evaluated GREEN at Task 4 (R6 STANDS WEAK-default; per-route resolution is O(1)). ADR-0194 NOT consumed; STAYS next-free. 1 NEW ADR §Decision + §Consequences body landed at the 22.3 IMPL Task 6 atomic landing (ADR-0193; §Context anchored at the 22.3 SPEC commit `e72af4c` per ADR-0044 in-place edit discipline) + 1 IN-PLACE AMENDMENT body landed on ADR-0125 §(xiv) (canonical roster 8 → 9; no new ADR number). ADR tail at ADR-0193 full body; ADR-0125 §(xiv) amended.

### envoy.filters.http.admission_control

Phase 23 ships `envoy.filters.http.admission_control` (Envoy v1.37.2 canonical SRE-book client-side admission-control filter — probabilistic request shedding over a sliding success-rate window; 503 on reject; both-sides decode-gate + encode-classify per AMEND-1/2/4/5/10/11) as the **SIXTEENTH §9 family-row** (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2, oauth2 @ 20, adaptive_concurrency @ 21, lua @ 22.1–22.3). Returns to the **LEAN framework-delta posture of phase 21** — ZERO new `internal/` framework primitives at the SPEC-time plan; ONE new framework primitive added at IMPL (ADR-0196 `ResponseStatus()` encode-side accessor — see encode-classify discipline below). **2 anchored ADRs** (ADR-0194 algorithm + package shape; ADR-0195 RTDS `runtime_key` PARSE-REJECT + `enabled`-absent-ENABLED semantics). **FIRST ADR-0125-skip since phase-22's roster amendment** (8 → 9 at 22.3); canonical-per-route roster STAYS 9 (REUSE-by-absence — the v1.37.2 admission_control proto has no `AdmissionControlPerRoute` message).

#### Algorithm — P_reject formula + integer-modulo decision (per ADR-0194 + AMEND-1/2)

Over a sliding `sampling_window` of per-second `{requests, successes}` buckets (a `std::deque`-of-per-second-buckets mirror), the filter computes a rejection probability and probabilistically short-circuits requests:

`P_reject = max(0, min(max_rejection_probability, ((n − s/sr_threshold) / (n + 1))^(1/aggression)))`

where `n = total requests in window`, `s = total successes`, `sr_threshold` = configured success-rate threshold (default 95%), `aggression` = configured aggression (default 1.0), `max_rejection_probability` = configured cap (default 80%). Pinned via ADR-0194 against `admission_control.cc:161-179`.

**Integer-modulo reject decision (per AMEND-2):** draw `r = Rand.Uint64()` (inline `Rand` interface seam — NOT `Float64()`); reject iff `(1e4 · P_reject) > (r % 1e4)`. This mirrors upstream's integer-truncation-to-10000 discipline. The `Rand` seam exposes `Uint64()` only; a production `cryptoRand` closure wraps `crypto/rand.Reader`.

**RPS-threshold gate (per AMEND-1):** if `window.averageRPS() < rps_threshold` (default 0.0), skip the formula and always admit — the `admission_control.cc:87-91` suppression gate mirrors upstream.

#### Both-sides decode-gate + encode-classify discipline (per AMEND-1/2/4/5/10/11)

**DecodeHeaders gate:**
1. `!filterEnabled()` (`enabled.default_value` honored; absent `enabled` = ENABLED per AMEND-4) → `record_request_ = false`; `Continue` (pass-through, not recorded).
2. `healthCheck()` arm — **NOT MODELED at MVP** (no `StreamInfo().IsHealthCheck()` accessor in envoy-go's `DecoderFilterCallbacks`; adding one is a new framework primitive — violates the ZERO-new-primitive constraint per PD-3). The AMEND-4 health-check short-circuit is structurally absent; AMEND-11's "health-check requests not recorded" is vacuous at MVP. Documented deferral — lifts when a `StreamInfo` accessor phase plumbs health-check flag.
3. `averageRPS() < rps_threshold` (AMEND-1 suppression gate) → `Continue` (admitted; `record_request_ = true`; no reject check).
4. `shouldRejectRequest()` — compute P_reject → integer-modulo decision → if reject: `record_request_ = false`; `rq_rejected.inc()`; `SendLocalReply(503, "", nil, nil, "denied_by_admission_control")`; `StopIteration`.
5. Otherwise (admit): `record_request_ = true`; `Continue`.

**EncodeHeaders classify (per AMEND-5/10/11):**
- If `record_request_ = false` (rejected / disabled / not-admitted-path) → no-op.
- HTTP classification: reads HTTP status via the `ResponseStatus()` framework accessor (per **ADR-0196** — NOT via a `:status` header read; the encode chain does not expose `:status` to filters; ADR-0196 added the `ResponseStatus() int` callback primitive to `EncoderFilterCallbacks` as the ONE new framework primitive introduced at phase-23 IMPL, superseding the SPEC-time PD-5 `:status`-via-header assumption). Default success gate: code `< 500` (per AMEND-5; `http_criteria.http_success_status` overrides).
- gRPC classification (per AMEND-10): if `Grpc.IsGrpcResponseHeaders(headers, end_stream)` → if gRPC status in headers, classify now; if gRPC status deferred to trailers, set `expect_grpc_status_in_trailer_ = true`. Default success set: 11-code well-known set `{OK,Cancelled,Unknown,InvalidArgument,NotFound,AlreadyExists,ResourceExhausted,FailedPrecondition,Aborted,Unimplemented,Unavailable}` (per AMEND-5; `grpc_criteria.grpc_success_status` overrides, up to 16 codes).
- On classification: `success → rq_success.inc(); recordSuccess()` / `failure → rq_failure.inc(); recordFailure()`.

**EncodeTrailers (per AMEND-10):** for the `expect_grpc_status_in_trailer_` case, read gRPC status from trailers and classify+record.

#### Reject wire shape (per AMEND-7 + D4)

- Status: `503 Service Unavailable`
- Body: **empty** (0 bytes; NOT a descriptive string — upstream's `sendLocalReply(503, "", ...)` byte-pinned)
- `response_code_details: "denied_by_admission_control"` (27-byte hard-coded literal; NO `RcDetails` constant in the filter; rc-details absent at the wire per **PD-2.503** — envoy-go MVP does not emit access-log `response_code_details`; the literal is internal-only)
- No filter-added response headers; no `grpc_status` passed (same 503/empty reply for gRPC and HTTP; no gRPC-aware reject branching)
- `rq_rejected` counter increments at the reject site immediately before `SendLocalReply`

#### 3-counter stat surface (per AMEND-3 + ADR-0194)

3 counters; NO gauges; NO histograms. Namespace `http.<HCM_stat_prefix>.admission_control.<stat>` HCM-rooted (mirrors phase-09 fault, phase-12 csrf, phase-14 compressor, phase-16 rbac, phase-17 jwt_authn, phase-18.1+18.2 ext_authz, phase-19.1+19.2 ext_proc, phase-20 oauth2, phase-21 adaptive_concurrency, phase-22 lua; DIVERGES from phase-15 bandwidth_limit's non-HCM-rooted shape). NO new SN flattening rule; SN2-reuse RATIFIED at SPEC §11 D1. All 3 unconditionally active under default config (NONE structurally unreachable). See `## Stat-name mapping` for the full table extension.

- `rq_rejected` — increments per request rejected by the probabilistic admit/reject decision.
- `rq_success` — increments per admitted request classified as a success at encode time.
- `rq_failure` — increments per admitted request classified as a failure at encode time. (**NOT** `rq_error` — AMEND-3 corrected the BRAINSTORM hypothesis via `ALL_ADMISSION_CONTROL_STATS(COUNTER)` macro at `admission_control.h:35-38`.)

Prometheus rendering via existing `envoy_http_conn_manager_prefix` SN2 extractor: `envoy_http_admission_control_<stat>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`.

#### Per-route discipline — REUSE-by-absence (per SPEC §5.4 + ADR-0195 §Consequences)

The v1.37.2 `AdmissionControl` proto has **no `AdmissionControlPerRoute` message at all**. Listener-scoped only. HCM-parse-time PARSE-REJECT for any admission_control `typed_per_filter_config` entry at route or virtualHost level (per ADR-0110 single-chokepoint; mirrors phase-20/21 REUSE-by-absence shape). **FIRST ADR-0125-skip since phase-22's roster amendment** (8 → 9 at 22.3); canonical-per-route roster STAYS 9. ADR-0195 records the explicit no-amendment classification. See `## Per-route canonical patterns cross-reference` below for the phase-23 cross-reference paragraph.

#### envoy-go-strict departure — RTDS `runtime_key` PARSE-REJECT (per ADR-0195; departure count 14 → 15)

Upstream Envoy v1.37.2 ACCEPTS a non-empty `runtime_key` inside any of the four `Runtime*` wrappers (`enabled.RuntimeFeatureFlag`, `aggression.RuntimeDouble`, `sr_threshold.RuntimePercent`/`max_rejection_probability.RuntimePercent`, `rps_threshold.RuntimeUInt32`) and consults the RTDS runtime layer at request time to override the static `default_value`. **envoy-go PARSE-REJECTs any non-empty `runtime_key`** in these four wrapper fields at HCM-parse-time with byte-stable wording (e.g., `"admission_control: enabled.runtime_key is not yet supported; use enabled.default_value"`). The static-default path (`default_value` honored) is the ONLY honored arm at MVP. Closes after the Runtime/RTDS family phase lands. Documented as the **SINGLE envoy-go-strict departure at phase 23** (departure count 14 → 15 — the 15th envoy-go-strict departure record across the §9 filter family).

**ADR-0196 `ResponseStatus()` framework primitive (encode-side accessor; phase 23 IMPL-time discovery):** The SPEC-time D-hypothesis predicted ADR-0196 would stay unconsumed; at IMPL the encode-side HTTP classification discovered that the encode chain does NOT expose `:status` as a header accessible to filters (PD-5's `:status`-via-header assumption was INVALID). ADR-0196 added the `ResponseStatus() int` accessor to `EncoderFilterCallbacks` + the `FilterChain.encodeResponseStatus` int field (set by HCM dispatch sites before `RunEncodeHeaders`). This is the ONE new framework primitive introduced at phase 23 (revising the "ZERO new framework primitives" plan claim). The admission_control `EncodeHeaders` reads `f.ecb.ResponseStatus()` for HTTP classification. Cross-phase reusable; future encode-side filters that need the upstream response status MAY use `ResponseStatus()` without further framework work.

### envoy.filters.http.ratelimit

Phase 24.1 ships the **CORE decision path** of `envoy.filters.http.ratelimit` (Envoy v1.37.2 canonical GLOBAL rate-limit filter — decode-side delegation of the rate-limit decision to an external `envoy.service.ratelimit.v3.RateLimitService/ShouldRateLimit` gRPC service driven by descriptors built from route/virtual-host `rate_limits` actions) as the **SEVENTEENTH §9 family-row** (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2, oauth2 @ 20, adaptive_concurrency @ 21, lua @ 22.1-22.3, admission_control @ 23). This SUBSECTION is the **24.1 PARTIAL BUNDLE** per parent SPEC §13: the CORE actions (5 of 10) + dispositions + cluster-scoped 4-counter surface + the DELTA-2 route-table exposure are LANDED HERE; the remaining 5 actions (`source_cluster`/`masked_remote_address`/`metadata`/`query_parameters`/`query_parameter_value_match`) + the X-RateLimit DRAFT_VERSION_03 response headers + `RateLimitPerRoute` (NEW 10th canonical + ADR-0125 amendment 9 → 10) + the `stage` multi-stage bucketing path + the Axis-B `vh_rate_limits` cross-tier composition will land at **24.2**. Phase 24.1 introduces **TWO framework deltas** (NOT framework-lean): DELTA-1 `internal/grpcclient/ratelimit_client.go` `RateLimitClient` (THIRD ADR-0158 typed wrapper after `AuthClient` + `ProcessorClient`) + DELTA-2 the HCM route-table `rate_limits` exposure (FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to an HTTP filter; chain-seeded `RouteRateLimits()`/`VirtualHostRateLimits()` `DecoderFilterCallbacks` accessor pair). **3 anchored ADRs** at 24.1 phase-done (ADR-0197[core] decode dispatch + dispositions + boot-registration; ADR-0198 the DELTA-2 route-table exposure; ADR-0200 the FULL §5 PARSE-REJECT roster). **ADR-0202 escape-valve UNCONSUMED at 24.1 phase-done** — the highest-risk byte-confirmation (the DELTA-2 chain-seed type + accessor return-type) RESOLVED as RAW-PROTO SEED CONFIRMED at Task 5 without firing the escape valve.

#### Decode-side request lifecycle (per ADR-0197[core] + AMEND-1/3/6/9/10/11)

**DecodeHeaders dispatch order:**
1. Build the descriptor list by walking the matched Route's `rate_limits[]` (filter-Axis A) — Axis B `vh_rate_limits` cross-tier composition lands at 24.2 — then for each policy entry, evaluate each `actions[]` entry in proto order via the **§4.1 5-CORE-action engine** (`generic_key`, `request_headers`, `remote_address`, `destination_cluster`, `header_value_match`). The **empty-action-drop discipline** per `router_ratelimit.cc:21-39`: if any action's evaluation returns `drop=true` (entry-missing) the ENTIRE descriptor is dropped (the policy entry produces no descriptor). At 24.1 the remaining 5 actions return drop=true (UNSUPPORTED-AT-24.1 arm — `actionUnsupportedAt241()`), which is structurally equivalent to "the action is not understood; drop the descriptor" — operators MUST scope policies to the 5 CORE actions until 24.2 lands the rest.
2. If the resulting descriptor list is empty (no policies, or all dropped), `Continue` the request immediately (no `ShouldRateLimit` dispatch).
3. Otherwise launch the async `ShouldRateLimit` gRPC dispatch on the per-stream `RateLimitClient` (per-request timeout from `compiledConfig.timeout`, default 20ms; OnDestroy cancels the per-stream `context.Context`); return `StopIteration` to park the decode dispatch goroutine.
4. The async goroutine routes on the gRPC response `overall_code` to ONE of the three disposition arms (`OK`/`OVER_LIMIT`/`error`), always wrapping the wire mutation with `ContinueDecoding()` to wake the parked dispatch goroutine (mirrors the ext_authz deny-path precedent + the phase-09 fault filter precedent — `SendLocalReply` alone sets `localReplyDone` but does NOT unblock `parkDecode`; the explicit `ContinueDecoding()` is required).

**OK disposition (per parent SPEC §4.7 + AMEND-1):** the request is admitted. `ok` counter `Inc`. The encode-side X-RateLimit response-header injection per the RLS `descriptor_statuses` is **STUBBED at 24.1 per D-RL7** — forward-pointer to 24.2 where `encode.go` + `headers.go` land. `ContinueDecoding()`.

**OVER_LIMIT disposition (per parent SPEC §4.7 + AMEND-8 header order):** the request is rejected. `over_limit` counter `Inc`. `SendLocalReply(cc.rateLimitedStatus, string(resp.RawBody), headers)` with the AMEND-8 header order: **[a]** `x-envoy-ratelimited: true` (UNLESS `enable_x_ratelimit_headers` is set to suppress it AND/OR the legacy `disable_x_envoy_ratelimited_header` boolean is true); **[b]** RLS-supplied `response_headers_to_add` in RLS-given order (empty-key entries are defensively skipped); **[c]** filter-config `response_headers_to_add` in config order. Status from `cc.rateLimitedStatus` (the 24.1 `rate_limited_status` field; default 429; below-400 values clamped to 429 at HCM-parse-time per Task 3). The `response_code_details = "request_rate_limited"` string is **ABSENT-BY-API at 24.1** (the 3-arg `SendLocalReply` callback does not surface rc-details to the wire per `internal/filter/http/callbacks.go` — same shape as the admission_control PD-2.503 disposition). The `rcDetailsRequestRateLimited` constant IS pinned in `dispositions.go` for forward-pointer 24.2 consumption when/if the API extends. gRPC-aware status mapping: when the upstream classifies as gRPC, the RLS code `OVER_LIMIT` maps to gRPC status `RESOURCE_EXHAUSTED=8` (or `UNAVAILABLE=14` when the deprecated `rate_limited_as_resource_exhausted=false` arm is honored — the typed helper `rateLimitedAsResourceExhaustedGrpcCode()` is pinned for forward-24.2-use, currently anchored at the 429-HTTP path). `ContinueDecoding()` after `SendLocalReply` wakes the parked decode goroutine.

**error disposition (per parent SPEC §4.7 + AMEND-1):** the RLS gRPC call errored (transport / timeout / OnDestroy-cancel) or returned `UNKNOWN`. `error` counter ALWAYS `Inc`. Then forked on the boolean `cc.failureModeDeny`:
- **`failure_mode_deny = false` (DEFAULT, fail-OPEN):** `failure_mode_allowed` counter `Inc` (additively, alongside `error`); `ContinueDecoding()` to admit the request. The upstream-default fail-open posture is preserved.
- **`failure_mode_deny = true` (fail-CLOSED):** `SendLocalReply(cc.statusOnError, "", nil)` — empty body, nil headers (the "nullptr-mutate" shape per upstream `ratelimit.cc` error codepath). Default `cc.statusOnError = 500`; below-400 values clamped to 500 at HCM-parse-time per Task 3. The `response_code_details = "rate_limiter_error"` is **ABSENT-BY-API** at 24.1 (same shape as OVER_LIMIT's rc-details). The `rcDetailsRateLimiterError` constant IS pinned in `dispositions.go` for forward-24.2 consumption. `ContinueDecoding()` after `SendLocalReply` wakes the parked decode goroutine.

#### 5-CORE-action descriptor engine (per parent SPEC §4.1 + ADR-0197[core])

The 5 CORE actions land at 24.1; the remaining 5 land at 24.2. Each action returns either `(entry, true, false)` (descriptor entry produced), `(nil, true, false)` (no-op — drop NOTHING; the descriptor is still produced from this policy's other entries), or `(nil, false, true)` (drop the ENTIRE descriptor — empty-action-drop):

| Action | Descriptor key | Descriptor value | 24.1 disposition |
|---|---|---|---|
| `generic_key` | `descriptor_key` (default `"generic_key"`) | the `descriptor_value` literal (REQUIRED) | LANDED (`actionGenericKey`) |
| `request_headers` | `descriptor_key` (REQUIRED) | the matched header's value (or drop if `header_name` absent AND `skip_if_absent=false`) | LANDED (`actionRequestHeaders`) |
| `remote_address` | `"remote_address"` (HARDCODED key) | downstream IP string from the chain-seeded `DownstreamRemoteAddr()` per ADR-0165 | LANDED (`actionRemoteAddress`) |
| `destination_cluster` | `"destination_cluster"` (HARDCODED key) | the matched route's upstream cluster name | LANDED (`actionDestinationCluster`); framework-limited to the matched cluster name (cluster-pick-time, not request-time per upstream) |
| `header_value_match` | `descriptor_key` (default `"header_match"`) | the `descriptor_value` literal | LANDED (`actionHeaderValueMatch`); supports all-of/none-of via per-request `HeaderMatcher` evaluation (Exact/Prefix/Suffix/Contains/SafeRegex; `Custom` arm NOT supported at 24.1 — falls through false) |
| `source_cluster` | — | — | UNSUPPORTED-AT-24.1 (returns drop=true; descriptor dropped); LANDS at 24.2 |
| `masked_remote_address` | — | — | UNSUPPORTED-AT-24.1; LANDS at 24.2 |
| `metadata` | — | — | UNSUPPORTED-AT-24.1; LANDS at 24.2 |
| `query_parameters` | — | — | UNSUPPORTED-AT-24.1; LANDS at 24.2 |
| `query_parameter_value_match` | — | — | UNSUPPORTED-AT-24.1; LANDS at 24.2 |
| `extension` | — | PARSE-REJECT at boot per ADR-0200 (envoy-go-strict departure; departure 17 → 17, see departures below) | NEVER LANDS (no descriptor-producer extension-point framework) |
| `dynamic_metadata` | — | PARSE-REJECT at boot per ADR-0200 (envoy-go-strict departure; deprecated upstream arm) | NEVER LANDS |

Header-matcher evaluation is per-request (NOT pre-compiled at HCM-parse-time, unlike the oauth2 precedent) — pre-compiling would require threading compiled state through the Task-5 chain seed (out of scope; the D-RL1 byte-confirmation sanctioned RAW-PROTO seed only). 24.2 MAY extract a pre-compile path if profiling surfaces the per-request `regexp.Compile` cost.

#### DELTA-2 — HCM route-table `rate_limits` exposure (per ADR-0198 + AMEND-9)

The 24.1 IMPL lands the FIRST framework exposure of route-level NON-`typed_per_filter_config` policy data to an HTTP filter. The HCM route table parses + retains the RAW `[]*envoy.config.route.v3.RateLimit` slices from each matched Route's `RouteAction.GetRateLimits()` AND the parent VirtualHost's `GetRateLimits()`; these are seeded onto the per-stream `FilterChain` at H1 (`internal/filter/hcm/connection.go`) + H2 (`internal/filter/hcm/h2dispatch.go`) dispatch sites via `chain.SetRouteRateLimits(...)` + `chain.SetVirtualHostRateLimits(...)` (set-once-by-dispatch / read-via-accessor; mirrors the ADR-0165 `DownstreamRemoteAddr` primitive); read by the filter via `DecoderFilterCallbacks.RouteRateLimits() []*routev3.RateLimit` + `DecoderFilterCallbacks.VirtualHostRateLimits() []*routev3.RateLimit`. Zero-value semantics: no-match-route / synthetic-stream / no-rate-limits paths all return `nil`. The **D-RL1 byte-confirmation** (parent SPEC §12 item 1 — HIGHEST RISK) RESOLVED at Task 5 as **RAW-PROTO SEED CONFIRMED**: the raw `[]*routev3.RateLimit` shape fit the existing ADR-0165 set-once primitive WITHOUT divergence; no pre-compiled carrier needed; no non-proto type needed. ADR-0202 escape-valve UNCONSUMED. PARSE-time validation is single-chokepoint via `ratelimit.ValidateRouteRateLimits` invoked from HCM `config.go` against each Route's + each VirtualHost's `rate_limits` (the FULL §5 PARSE-REJECT roster including `disable_key`/`extension`/`dynamic_metadata` envoy-go-strict departures fires here).

#### Cluster-scoped 4-counter stat surface (per AMEND-1 + AMEND-10 + ADR-0197[core])

4 counters; NO gauges; NO histograms. Namespace **`cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>`** CLUSTER-rooted (NOT HCM-rooted; this is the **FIRST cluster-scoped cross-namespace stat write** by an HTTP filter — LANDS the pattern that ext_authz's `charge_cluster_response_stats` DEFERRED). DIVERGES from every other phase-09..23 §9 family-row's HCM-rooted shape. The `<rls_cluster_name>` is the upstream RLS cluster name captured at HCM-parse-time from `rate_limit_service.grpc_service.envoy_grpc.cluster_name`; the OPTIONAL `<stat_prefix>` segment (the 24.1 `compiledConfig.statPrefix` AMEND-3 13th field) is elided WHOLESALE (including its leading dot) when empty. Registration uses `(*stats.Registry).NewCounterIfAbsent` (the POST-Freeze-safe idempotent path per ADR-0117) — load-bearing across MULTIPLE listeners sharing one RLS cluster (each filter-instance gets the SAME `*Counter` handle for each leaf name; charges from all instances aggregate into the same atomic cell). ADR-0085 nil-tolerance: when `reg == nil` (test code paths without a Registry) the constructor returns a non-nil `*filterStats` with all-nil counter fields; the disposition path nil-guards each `Inc()` call. All 4 counters are registered UNCONDITIONALLY at `New()` time per the established filter-stats unconditional-registration discipline.

- `ok` — RLS returned `overall_code = OK` (admit-the-request).
- `error` — `ShouldRateLimit` gRPC call returned a transport/RLS error (timeout / network / OnDestroy-cancel / `UNKNOWN`); ALWAYS increments on the error arm (additively with `failure_mode_allowed` on the fail-open path).
- `over_limit` — RLS returned `overall_code = OVER_LIMIT` (reject-the-request).
- `failure_mode_allowed` — error arm AND `failure_mode_deny=false` (fail-OPEN admit). Increments ALONGSIDE `error` on the error-fail-open path; NOT incremented on the error-fail-closed path.

Per-route stats: SHARED with listener-level at 24.1 (vacuously — the per-route `RateLimitPerRoute` proto + the `RateLimitPerRoute.domain` override LAND at 24.2 along with the NEW 10th canonical per ADR-0125 §(xv) amendment; 24.1 sees no per-route domain override). See `## Stat-name mapping` for the full table extension.

#### X-RateLimit response headers — STUBBED at 24.1 per D-RL7 (forward-pointer to 24.2)

The X-RateLimit DRAFT_VERSION_03 response-header injection (`x-ratelimit-limit` / `x-ratelimit-remaining` / `x-ratelimit-reset` with MIN-status + `;w=<window>` / `;name=<descriptor>` quota suffix, emitted on ALL dispositions when `enable_x_ratelimit_headers` selects DRAFT_VERSION_03) is **STUBBED at 24.1 per the D-RL7 24.1/24.2 split (parent SPEC §16 axis)**. At 24.1 the `EncodeHeaders` arm is a pass-through `Continue` (the filter stores `f.ecb` for forward-24.2-use; the `//nolint:unused` annotation is the deliberate placeholder). The header layout, MIN-status across multi-descriptor responses, and the legacy `enable_x_ratelimit_headers=OFF` no-op behavior all land at 24.2 along with the encode-side filter body.

#### Cross-references

- **ADR-0197[core]** — §Decision + §Consequences FULL (24.1 slice) at Task 7 — package shape + dispatch + dispositions + boot-registration + 19 HTTP filters. The X-RateLimit / encode-side slice lands at 24.2 (ADR-0197 will be amended in-place at 24.2's encode-side Task).
- **ADR-0198** — §Decision + §Consequences FULL at Task 5 — DELTA-2 HCM route-table `rate_limits` exposure framework capability + the `RouteRateLimits()`/`VirtualHostRateLimits()` chain-seeded accessor pair + the RAW-PROTO seed type byte-confirmation (D-RL1 resolved).
- **ADR-0200** — §Decision + §Consequences FULL at Task 3 — the FULL §5 PARSE-REJECT roster (the RATIFIED-from-PGV/config arms + the 3 envoy-go-strict departures `disable_key`/`extension`/`dynamic_metadata`).
- **ADR-0202** — UNCONSUMED at 24.1 phase-done (escape-valve reserve for the D-RL1 chain-seed divergence; did not fire).
- **AMEND-1** — stats anchored at CLUSTER scope (NOT HCM); 4 counters (`ok`/`error`/`over_limit`/`failure_mode_allowed`); 110 → 114 (see `## Stat-name mapping` extension).
- **AMEND-3** — 13-field `compiledConfig` roster + defaults/clamps (`rate_limited_status` 429 default + <400 clamp-to-429; `status_on_error` 500 default + <400 clamp-to-500; `timeout` 20ms default; `stage` 0 default; `failure_mode_deny` false default; `disable_x_envoy_ratelimited_header` false default; etc.).
- **AMEND-6** — descriptor `hits_addend` is `UInt64Value` + non-monotonic Unit enum (the fake RLS service encodes by proto NUMBER); 24.1 honors the receive path; the send-side `hits_addend` is part of the §4.1 action engine's descriptor production (not in the 5 CORE actions; LANDS at 24.2 with the `metadata` action's hits-addend variant).
- **AMEND-8** — OVER_LIMIT header order `[x-envoy-ratelimited]→[RLS response_headers_to_add]→[config response_headers_to_add]` (the first slot is suppressed when `disable_x_envoy_ratelimited_header=true`).
- **AMEND-9** — DELTA-2 is GENUINELY NEW (NOT a `RequestRouteConfig()` reuse — TPFC is keyed by filter NAME; `rate_limits` are first-class fields unparsed today). HIGH risk; D-RL1 byte-confirmation RESOLVED as RAW-PROTO SEED CONFIRMED.
- **AMEND-10** — cluster-scoped cross-namespace stat write via `NewCounterIfAbsent` (LANDS the pattern ext_authz's `charge_cluster_response_stats` DEFERRED).
- **AMEND-11** — boot-registration: 18 → 19 HTTP filters (alphabetical between `oauth2` and `rbac`).

**ADR-0202 escape-valve disposition (24.1 phase-done):** UNCONSUMED. The D-RL1 byte-confirmation RESOLVED as RAW-PROTO SEED CONFIRMED at Task 5; the raw `[]*routev3.RateLimit` shape fit the existing ADR-0165 `DownstreamRemoteAddr` set-once primitive WITHOUT divergence. The conditional escape-valve obligation passes through to any later phase-24.x task that might surface a divergence (none anticipated; 24.2's per-route + X-RateLimit slices do not touch the chain-seed primitive).

#### envoy-go-strict departure — `disable_key` non-empty PARSE-REJECT (per ADR-0200; departure count 15 → 16)

Upstream Envoy v1.37.2 ACCEPTS a non-empty `disable_key` on a `route.RateLimit` policy entry (the policy is GATED at request-time by the `runtime_key` lookup against RTDS) and silently skips the policy when the runtime key resolves to a disabled state. **envoy-go PARSE-REJECTs any non-empty `disable_key`** at HCM-parse-time per ADR-0200 with byte-stable wording (`"ratelimit: disable_key is not yet supported (use omit or empty)"`) — invoked via the single-chokepoint `ratelimit.ValidateRouteRateLimits` from HCM `config.go` against each Route's + each VirtualHost's `rate_limits`. The static-policy path (`disable_key` empty/absent — the policy is ALWAYS evaluated) is the ONLY honored arm at 24.1. Closes when the Runtime/RTDS family phase lands. Documented as **departure 15 → 16** — the 16th envoy-go-strict departure record across the §9 filter family (1st of 3 phase-24.1 records).

#### envoy-go-strict departure — `extension` action PARSE-REJECT (per ADR-0200; departure count 16 → 17)

Upstream Envoy v1.37.2 ACCEPTS the descriptor-action `oneof action_specifier = extension(TypedExtensionConfig)` arm, dispatching to any registered descriptor-producer extension at boot/request time. **envoy-go PARSE-REJECTs any `extension` action** at HCM-parse-time per ADR-0200 with byte-stable wording (`"ratelimit: action 'extension' is not yet supported (no descriptor-producer extension-point framework)"`) — no descriptor-producer extension-point framework exists in envoy-go MVP. The 10 well-known `action_specifier` arms are the entire honored surface (5 at 24.1, 5 more at 24.2). Closes when (if) a descriptor-producer extension framework is introduced. Documented as **departure 16 → 17** — the 17th envoy-go-strict departure record (2nd of 3 phase-24.1 records).

#### envoy-go-strict departure — `dynamic_metadata` action PARSE-REJECT (per ADR-0200; departure count 17 → 18)

Upstream Envoy v1.37.2 STILL SUPPORTS the deprecated `oneof action_specifier = dynamic_metadata(DynamicMetaData)` arm (DEPRECATED in favor of the `metadata` action which subsumes its surface) — the deprecated arm continues to function. **envoy-go PARSE-REJECTs any `dynamic_metadata` action** at HCM-parse-time per ADR-0200 with byte-stable wording (`"ratelimit: action 'dynamic_metadata' is not yet supported (use metadata instead)"`) — operators are funneled to the canonical `metadata` arm (which itself lands at 24.2). At 24.1 BOTH arms are unavailable (`metadata` is UNSUPPORTED-AT-24.1; `dynamic_metadata` is PERMANENTLY-REJECTED). Documented as **departure 17 → 18** — the 18th envoy-go-strict departure record (3rd of 3 phase-24.1 records). Phase 24.2 closes the `metadata` half; `dynamic_metadata` stays permanently rejected (deprecated upstream).

#### Phase 24.2 completion bundle (per D-RL17 + parent §13 — atomic landing per ADR-0052)

Phase 24.2 closes the 24.1 partial slice: the remaining 5 actions land + the X-RateLimit DRAFT_VERSION_03 response-header emission lands + the `RateLimitPerRoute` 10th canonical lands (ADR-0125 §(xv) AMENDMENT 9 → 10 LANDED at Task 3) + the `stage` multi-stage bucketing path lands + the Axis-B `vh_rate_limits` cross-tier composition table lands + the legacy `RouteAction.include_vh_rate_limits=true` force-include arm lands. **NO new departure records** — the 3 phase-24.1 departure records (`disable_key` + `extension` + `dynamic_metadata`) cover the entire envoy-go-strict departure surface; `override_option` accepted-but-IGNORED is upstream-parity (NOT a departure). Departure count STAYS at 18 after phase 24.2.

**Descriptor engine — completion to all 10 actions (per AMEND-11 + parent SPEC §4.1).** Phase 24.2 Task 1 lands the 5 remaining `oneof action_specifier` arms. Each action returns either `(entry, true, false)` (descriptor entry produced), `(nil, true, false)` (no-op — drop NOTHING; the descriptor is still produced from this policy's other entries), or `(nil, false, true)` (drop the ENTIRE descriptor — empty-action-drop):

| Action (24.2 lands) | Descriptor key | Descriptor value | 24.2 disposition |
|---|---|---|---|
| `source_cluster` | `"source_cluster"` (HARDCODED) | the Envoy NODE's `service-cluster` name from the chain-seeded `NodeServiceCluster` factory-context field (set at HCM bootstrap from `Node.cluster` per `bootstrap.proto`); empty string ⇒ drop=true (entry-missing) | LANDED (`actionSourceCluster`); framework-limited to the bootstrap NODE identity (NOT per-request) |
| `masked_remote_address` | `"masked_remote_address"` (HARDCODED) | downstream IP/PREFIX (v4 default mask 32; v6 default mask 128; per-action overridable via the `v4_prefix_mask_len`/`v6_prefix_mask_len` UInt32Value fields); netmask applied to the chain-seeded `DownstreamRemoteAddr()` per ADR-0165 | LANDED (`actionMaskedRemoteAddress`); v4/v6 prefix-mask discipline byte-pinned at the 24.2 Task-1 + fixture (d)-extension cross-side dispatch |
| `metadata` | `descriptor_key` (REQUIRED) | the segmented `MetadataKey.path` descent through `*structpb.Value` per `Metadata::metadataValue` upstream reference at `source/common/config/metadata.cc`; D-RL8 RESOLVED: `MetadataSource_DYNAMIC=0` via the existing `DecoderFilterCallbacks.DynamicMetadata()` accessor; `MetadataSource_ROUTE_ENTRY=1` via the NEW `DecoderFilterCallbacks.RouteMetadata()` accessor (added at 24.2 Task 1 per the ADR-0165 set-once-by-dispatch extension template — chain-field + setter + chain-accessor + decoderCB accessor + HCM-dispatch seed at H1 + H2). `default_value` falls back when the descent fails; `skip_if_absent` controls drop-vs-no-op | LANDED (`actionMetadata`); **D-RL8 byte-confirmation outcome: RESOLVED CLEANLY — ADR-0202 UNCONSUMED** at Task 1; the existing `DynamicMetadata()` + the NEW `RouteMetadata()` plumbing fit the existing primitives without divergence |
| `query_parameters` | `descriptor_key` (default `"query_param"` SINGULAR per AMEND-11; NOT the plural `"query_parameters"` BRAINSTORM-anticipated) | the matched query-parameter's value (or drop if `query_parameter_name` absent AND `skip_if_absent=false`) | LANDED (`actionQueryParameters`); the default-key value is AMEND-11 corrected against the BRAINSTORM hypothesis (the upstream `router_ratelimit.cc:174` defaults to SINGULAR `"query_param"`) |
| `query_parameter_value_match` | `descriptor_key` (default `"query_match"`) | the `descriptor_value` literal | LANDED (`actionQueryParameterValueMatch`); supports all-of/none-of via per-request `QueryParameterMatcher` evaluation (Value/Present matchers; mirrors the 24.1 `header_value_match` action's per-request matcher discipline) |

The `actionUnsupportedAt241()` arm is DELETED at 24.2 Task 1 (the 5 remaining actions all dispatch cleanly through their now-implemented arms). The empty-action-drop discipline (per `router_ratelimit.cc:21-39`) continues to fire at the `buildDescriptorForPolicy` level on entry-missing returns. **All 10 framework-reachable actions LANDED at 24.2;** the `extension` + `dynamic_metadata` arms continue to PARSE-REJECT at HCM-parse-time per the 24.1-landed ADR-0200 departure records.

**`destination_cluster` framework-limited disposition (per 24.1 PROGRESS Task 6).** The 24.1-landed `actionDestinationCluster` retrieves the matched-route's upstream cluster name (cluster-pick-time, NOT request-time). Upstream Envoy's `Router::ClusterDiscoveryStatus`-aware request-time cluster resolution (which can change between match-time and request-time under request-time cluster-discovery service interactions) is NOT implemented at envoy-go MVP — `destination_cluster` resolves at match-time only. This is a framework limitation (NOT an envoy-go-strict departure — the descriptor produced is byte-exact upstream for the static cluster-match case; the runtime-discovery diff only manifests for CDS+xDS dynamic cluster phases that envoy-go MVP does not implement).

**Stage multi-bucket discipline (per parent §4.4 + 24.2 Task 2).** The `stage` field is parsed + clamped to [0, 10] at HCM-parse-time (24.1-landed; default 0; `>10` PARSE-REJECTs per AMEND-3); at 24.2 Task 2 the **per-stage bucketing path** lands. The descriptor engine walks `rate_limits[]` ONCE per request but groups the policies by stage (slots 0..10) via `bucketRateLimitsByStage`; each stage-bucket emits its OWN `ShouldRateLimit` request to the RLS service (multiple stages ⇒ multiple gRPC calls in sequence, NOT a single batched call — the upstream serializes by stage). At MVP the per-policy `stage` defaults to 0 (single-bucket) so the multi-bucket arm is structurally inactive unless operators set per-policy `stage` ≠ 0; the multi-bucket arm is covered by Task 2's `bucketRateLimitsByStage_test.go` + the 24.2 Task-7 corpus seeds (stage=5 valid arm + stage=11 PARSE-REJECT arm). The first-stage OVER_LIMIT short-circuits subsequent stages (no dispatch to later stages on early reject).

**Axis-B `vh_rate_limits` cross-tier composition table (per parent §4.3 + AMEND-5 + 24.2 Task 4).** The decision-table for which `rate_limits[]` policies fire per request:

| Per-route present? | `RateLimitPerRoute.rate_limits[]` non-empty? | `vh_rate_limits` enum (or DEFAULT=OVERRIDE) | Legacy `RouteAction.include_vh_rate_limits=true`? | Effective walk |
|---|---|---|---|---|
| no | n/a | n/a | no | Route.rate_limits[] only (24.1-shape; Axis-A only) |
| no | n/a | n/a | YES | Route.rate_limits[] + VirtualHost.rate_limits[] (legacy force-include arm; the legacy bool wins) |
| yes | yes (non-empty) | n/a | n/a | per-route `rate_limits[]` ONLY (Axis-A EARLY-RETURN per AMEND-4; ignores Route + VH) |
| yes | no (empty) | OVERRIDE (default) | no | Route.rate_limits[] only (per-route is metadata-only with no override; VH is OVERRIDDEN) |
| yes | no (empty) | OVERRIDE (default) | YES | Route.rate_limits[] + VirtualHost.rate_limits[] (legacy bool wins; OVERRIDE is countermanded) |
| yes | no (empty) | INCLUDE | n/a | Route.rate_limits[] + VirtualHost.rate_limits[] |
| yes | no (empty) | IGNORE | n/a | Route.rate_limits[] only (VH ignored even with legacy bool true — IGNORE supersedes the legacy bool) |

The `override_option` field on `RateLimitPerRoute` (an OPTION enum that upstream-protos as `[#not-implemented-hide:]`) is PARSE-ACCEPTED-but-IGNORED at all arms (NEVER consulted at request time per AMEND-4). The per-route `domain` field is consulted at request time (when set, the descriptor's `domain` is the per-route value; when absent, the filter-config `domain` applies). **D-RL11 lock (parent §4.3 + AMEND-4 + AMEND-5):** Axis-A EARLY-RETURN takes precedence over Axis-B; INCLUDE/IGNORE/OVERRIDE are honored in proto order against the per-route metadata; legacy `include_vh_rate_limits=true` is consulted ONLY when the per-route inclusion enum says OVERRIDE (DEFAULT or explicit); IGNORE supersedes the legacy bool (the IGNORE intent is stronger than the legacy intent).

**X-RateLimit DRAFT_VERSION_03 emission discipline (per parent SPEC §4.7 + AMEND-8 + 24.2 Task 5 + ADR-0197 in-place amendment).** When `enable_x_ratelimit_headers == DRAFT_VERSION_03` (the 24.1 `compiledConfig.enableXRatelimitHeaders` AMEND-3 field), the encode hook injects three response headers on ALL non-fail-closed dispositions (OK + OVER_LIMIT + fail-OPEN error):
- `x-ratelimit-limit: <MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` — MIN selection by `limit_remaining`; quota-policy suffix per descriptor with non-zero window (`Unit→seconds`: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000; UNKNOWN/0 ⇒ no quota-policy segment for that descriptor); `;name=` value quoted per upstream `ratelimit_headers.cc:13-65` reference.
- `x-ratelimit-remaining: <MIN.limit_remaining>` — integer-ASCII.
- `x-ratelimit-reset: <MIN.duration_until_reset.seconds>` — integer-ASCII (full seconds; fractional seconds NOT honored at 24.2 per the byte-format upstream).

**Fail-closed disposition does NOT emit X-RateLimit headers** (`failure_mode_deny=true` + RLS error ⇒ `SendLocalReply` with nullptr-mutate per AMEND-8; the encode hook does not participate). **Wire order on OVER_LIMIT (per AMEND-8 + 24.2 Task-5 follow-up I-1 fix):** the X-RateLimit headers are injected by `applyOverLimit` AT the `SendLocalReply` site BETWEEN slot [a] `x-envoy-ratelimited: true` (unless suppressed) and slot [c] filter-config `response_headers_to_add` — i.e., the [a]+[X-RateLimit]+[b RLS response_headers_to_add]+[c] order — per parent SPEC §4.7 line 214. The 24.2 Task-5 follow-up (commit `ce8ca49`) corrected the initial wire-order regression that placed X-RateLimit AFTER slot [c]. **MIN-selection tie-breakers** (equal `limit_remaining`): preserve insertion order (= descriptor-list order = action-list order per AMEND-6) — the FIRST equal-minimum status wins.

**`RateLimitPerRoute` 10th canonical (per ADR-0199 + ADR-0125 §(xv) + 24.2 Task 3).** The `RateLimitPerRoute` TPFC entry lands as the **NEW 10th canonical per-route shape** (ADR-0125 §(xv) AMENDMENT 9 → 10 LANDED at 24.2 Task 3). The shape: `data-only-with-vh-inclusion-enum` — 4 fields (`vh_rate_limits` inclusion enum + `rate_limits[]` route-additional Axis-A + `override_option` accepted-but-IGNORED + `domain` descriptor-tier override) — all of which are DATA carriers (NOT disabled-bool, NOT string-reference, NOT wholesale-override sub-message). Boot-time validation single-chokepoint via `validatePerRouteRateLimit` per ADR-0110; the validator recursively invokes `ValidateRouteRateLimits` against the embedded `rate_limits[]` so the FULL §5 PARSE-REJECT roster fires at per-route TPFC compile time. Request-time projection via `compilePerRouteForRequest` at the filter callback site. The TPFC TypeURL `"type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimitPerRoute"` is registered alongside the existing per-route TPFC entries in `internal/filter/hcm/` per ADR-0073.

**`RateLimitPerRoute.override_option` departure note (NOT an envoy-go-strict departure per AMEND-4).** The `override_option` field is upstream-tagged `[#not-implemented-hide:]` — upstream Envoy v1.37.2 ACCEPTS the field at parse time but NEVER consults it at request time. envoy-go honors this upstream-parity decision: the field is PARSE-ACCEPTED (no PARSE-REJECT — operators can set it without boot failure) but NEVER consulted at request time. **NOT a departure record** (no divergence from upstream behavior; both sides ignore the field at runtime). Documented here so operators understand the field's accepted-but-INERT status without searching the upstream source.

**Cross-references (24.2 additions):**
- **ADR-0197 (in-place §Decision amendment at Task 5)** — the X-RateLimit + remaining-actions slice. The 24.1 §Decision body covered the CORE decision path; the 24.2 in-place amendment per ADR-0052 closes the X-RateLimit DRAFT_VERSION_03 emission + the AMEND-8 wire order (with the Task-5 follow-up I-1 fix) + the 5 remaining actions' descriptor production.
- **ADR-0199 (§Decision + §Consequences FULL at Task 3)** — `RateLimitPerRoute` 10th canonical + ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph LANDED.
- **AMEND-4** — `override_option` accepted-but-IGNORED + embedded `rate_limits` early-return + per-route `domain` override.
- **AMEND-5** — legacy `RouteAction.include_vh_rate_limits=true` force-include arm honored at the Axis-B decision table (gated by the per-route inclusion enum's OVERRIDE arm).
- **AMEND-8** — X-RateLimit emission wire-order at the OVER_LIMIT [a]+[X-RateLimit]+[b]+[c] slot (the 24.2 Task-5 follow-up I-1 fix corrected the initial regression).
- **AMEND-11** — descriptor-action default-key roster ratified: `generic_key` default `"generic_key"` / `header_value_match` default `"header_match"` / `query_parameters` default `"query_param"` SINGULAR / `query_parameter_value_match` default `"query_match"` / `source_cluster` + `destination_cluster` + `remote_address` + `masked_remote_address` HARDCODED keys / `request_headers` REQUIRED `descriptor_key` / `metadata` REQUIRED `descriptor_key`.

**ADR-0202 escape-valve disposition (24.2 phase-done):** UNCONSUMED. The two byte-confirmation surfaces (D-RL8 metadata accessor at Task 1; D-RL9 X-RateLimit byte format at Task 5) RESOLVED CLEANLY at IMPL: D-RL8 via the existing `DynamicMetadata()` + the NEW `RouteMetadata()` accessor (ADR-0165 set-once extension template); D-RL9 via byte-exact reproduction of the upstream `ratelimit_headers.cc` byte format pinned at `headers_test.go` against captured upstream headers AND verified cross-side byte-exact at fixture `0032-http-ratelimit` scenario (g). **D-hypothesis HELD across the entire phase 24** (phase-24.1 + phase-24.2 BOTH UNCONSUMED ADR-0202). Next-free ADR stays at `ADR-0202`.

### envoy.filters.http.wasm

Phase 25.1 ships the **headers-only foundational third** of `envoy.filters.http.wasm` (Envoy v1.37.2 canonical HTTP WebAssembly filter — loads operator-authored `.wasm` modules under a default-deny proxy-wasm v0.2.1 sandbox at HCM dispatch; per-stream `*wasm.VM` construction; dispatches `proxy_on_request_headers` + `proxy_on_response_headers` against a 24-hostcall + 13-callback bridge surface) as the **EIGHTEENTH and FINAL §9 family-row** (after cors @ 07.1, fault @ 09, header_mutation @ 10, local_ratelimit @ 11, csrf @ 12, buffer @ 13, compressor @ 14, bandwidth_limit @ 15, rbac @ 16, jwt_authn @ 17, ext_authz @ 18.1+18.2, ext_proc @ 19.1+19.2, oauth2 @ 20, adaptive_concurrency @ 21, lua @ 22.1–22.3, admission_control @ 23, ratelimit @ 24.1+24.2). FIRST §9 family-row to (i) **delegate per-request behavior to operator-authored compiled-to-WebAssembly modules** (the SECOND §9 row after lua to delegate to operator-authored code in any form; first to use a compiled wire format), (ii) introduce a NEW framework primitive of substantial scope (`internal/wasm/` per ADR-0202 — the SECOND occurrence of EXTRACT-NOW-at-first-consumer after phase-22.1 `internal/lua/`), and (iii) introduce a WebAssembly runtime dependency (`github.com/tetratelabs/wazero` v1.10.1 — pure-Go WebAssembly Core 1.0+2.0; Apache-2.0-licensed; CNCF Sandbox project; NO CGO). **3 anchored ADRs at phase 25.1** (ADR-0202 NEW `internal/wasm/` framework primitive — VM lifecycle + per-stream `*wazero.Runtime` + per-module `*Module` compile cache + `SandboxConfig` zero-value `StrictDefaultDeny` posture per AMEND-A5 + in-house proxy-wasm v0.2.1 host ABI implementation + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2; ADR-0203 NEW `internal/filter/http/wasm/` package shape — `compiledConfig` + 5-counter `filterStats` per AMEND-A2 + 18-arm PARSE-REJECT roster + 4-arm AsyncDataSource.Local + 24-hostcall + 13-callback bridge surface + fixture-0034 + fixture-0035; ADR-0204 proxy-wasm capability-restriction default-deny + envoy-go-strict sandbox posture). **D-P4 R8 disposition: STANDS WEAK-default** — per-stream `*wasm.VM` construction measured at `ns/op = 61000` (~61µs) << 1ms threshold; ADR-0205 NOT consumed; per-stream construction posture retained (no per-module wazero Runtime pool); carries forward to 25.2 BRAINSTORM as the 25.2 IMPL escape-valve slot. **3 envoy-go-strict departure records** land at this 25.1 IMPL bundle: default-deny capability sandbox (per AMEND-A5 + ADR-0204); ABI v0.1.0 + v0.2.0 PARSE-REJECT (per AMEND-A6); consolidated departure bundle (`AsyncDataSource.Remote` PARSE-REJECT + runtime-name discriminator PARSE-REJECT + 3 envoy-go-strict counters).

#### Field decomposition

**Listener-level `envoy.extensions.filters.http.wasm.v3.Wasm` (carries `PluginConfig`) + `PluginConfig` (carries `VmConfig` + sandbox + plugin envelope) + `VmConfig` (carries the wasm bytecode + runtime selector):**

| Proto field | envoy-go phase-25.1 disposition |
|---|---|
| `Wasm.config` (`PluginConfig`) | CONSUMED — top-level wrapper carrying the plugin envelope. PARSE-REJECT on nil per arm 1. |
| `PluginConfig.name` (`string`) | CONSUMED — discriminator for envoy-go-strict per-plugin counter namespace (`wasm.<plugin_name>.*` per AMEND-A2). Empty-name produces literal consecutive-dot wire names (`wasm..executions`) — mirrors phase-22.1 empty-`<config_stat_prefix>` precedent; explicit empty-name PARSE-REJECT deferred (boot-time-fail-fast on `*stats.Registry` collision provides the operator-visible signal). |
| `PluginConfig.root_id` (`string`) | CONSUMED — string handle passed through to the wasm guest at `proxy_on_context_create(rootCtxID=1, 0)` lifecycle; honored verbatim per the proxy-wasm v0.2.1 spec. |
| `PluginConfig.vm_config` (`VmConfig`) | CONSUMED — required at 25.1 (PARSE-REJECT on nil per arm 2). Carries the wasm bytecode + runtime selector + the `vm_id` singleton-VM-key (multi-plugin-per-VM via `vm_id` sharing DEFERRED to 25.3 per parent §3.0). |
| `PluginConfig.configuration` (`google.protobuf.Any`) | CONSUMED-VERBATIM — opaque bytes forwarded to the wasm guest at `proxy_on_configure(rootCtxID, configSize)` lifecycle. NOT parsed at envoy-go side. |
| `PluginConfig.capability_restriction_config` (`CapabilityRestrictionConfig`) | CONSUMED — `allowed_capabilities` map keys gate the per-capability sandbox via `SandboxConfig.IsAllowed(<name>)` per ADR-0204 + AMEND-A5. **Empty map → DENY-ALL (envoy-go-strict departure from upstream allow-all — see departure record #1 below).** |
| `PluginConfig.fail_open` (`BoolValue`) | DEFERRED to 25.3 — `failure_policy = FAIL_RELOAD` reload disposition lifts at 25.3 with the per-module VM-reload state machine. |
| `PluginConfig.allow_precompiled` (`bool`) | DEFERRED — at 25.1 envoy-go uses wazero's interpreter-mode default; precompiled-cache disposition is a future opt-in. |
| `PluginConfig.nack_on_code_cache_miss` (`bool`) | DEFERRED — coupled to the precompiled-cache surface above. |
| `VmConfig.vm_id` (`string`) | CONSUMED-as-singleton at 25.1 — each `PluginConfig` constructs a fresh per-stream VM; cross-plugin VM-sharing via shared `vm_id` DEFERRED to 25.3 per parent §3.0. |
| `VmConfig.runtime` (`string`) | CONSUMED with PARSE-REJECT discipline per arm 11 — only `""` (default) and `"envoy.wasm.runtime.wazero"` accepted. The 3 alternative discriminators (`envoy.wasm.runtime.v8`/`wamr`/`wasmtime`) PARSE-REJECT per AMEND-A2 + departure record #3 (envoy-go ships only wazero per AMEND-A1). |
| `VmConfig.code` (`AsyncDataSource`) | CONSUMED — required (PARSE-REJECT on nil per arm 3). 4-arm `AsyncDataSource.Local` resolution per `core.DataSource` (Filename + InlineBytes + InlineString + EnvironmentVariable). `AsyncDataSource.Remote` arm PARSE-REJECT per arm 6 + departure record #3 (envoy-go-strict — upstream supports remote bytecode fetching via the runtime/RTDS surface). |
| `VmConfig.configuration` (`google.protobuf.Any`) | CONSUMED-VERBATIM — opaque bytes forwarded to the wasm guest at `proxy_on_vm_start(rootCtxID, vmConfigSize)` lifecycle. |
| `VmConfig.environment_variables` (`EnvironmentVariables`) | DEFERRED to 25.3 — env-var inject envelope honored at VM start lands at 25.3 alongside the per-route + multi-plugin sub-phase. At 25.1 the WASI `environ_get` / `environ_sizes_get` shims return zero entries. |
| `core.DataSource.specifier.filename` | CONSUMED. PARSE-REJECT on name-empty / ENOENT / zero-byte (arms 7-9). |
| `core.DataSource.specifier.inline_bytes` | CONSUMED. PARSE-REJECT on empty (arm 10). |
| `core.DataSource.specifier.inline_string` | CONSUMED. PARSE-REJECT on empty (arm 12). |
| `core.DataSource.specifier.environment_variable` | CONSUMED via `os.LookupEnv`. PARSE-REJECT on name-empty / unset / empty-value (arms 13-15). |
| `core.DataSource.watched_directory` | PARSE-REJECT per arm 5 — deferred to a future Runtime/hot-reload phase. Mirrors phase-22.1 lua arm-7 PARSE-REJECT pattern. |
| `WasmPerRoute` (any `typed_per_filter_config` entry) | **PARSE-REJECT-by-absence** per AMEND-A3 — the v1.37.2 + go-control-plane v1.32.4 proto roster surfaces NO `WasmPerRoute` message. ADR-0125 STAYS at 10 canonicals. Per-route shape settled at 25.3 as the 5th-canonical REUSE-by-absence (mirrors phase-20 oauth2 + phase-21 adaptive_concurrency + phase-23 admission_control absence-strongest-form pattern). |

#### 18-arm PARSE-REJECT roster (parent §6.2 + 25.1 IMPL byte-stable wording finalization per D-P5)

Per parent SPEC §6.2 + AMEND-A6 (ABI version envoy-go-strict-stricter) + Task 9 D-P5 byte-stable wording closure. Byte-stable wording per ADR-0080 + the phase-22.1 + phase-23 + phase-24.1 SPEC §6 precedent. Constants live as package-private `parseReject*` consts at `internal/filter/http/wasm/compiled_config.go` + `datasource.go`. All wording is byte-pinned via the package's `compiled_config_test.go::TestParseRejectConstants_ByteStable` table-driven assertion.

| Arm | Trigger | Wording (byte-exact) |
|---|---|---|
| 1 | `Wasm.config` nil | `wasm: config required` |
| 2 | `Wasm.config.vm_config` nil | `wasm: config.vm_config required` |
| 3 | `Wasm.config.vm_config.code` nil | `wasm: config.vm_config.code required` |
| 4 | `Wasm.config.vm_config.code.specifier` empty oneof | `wasm: config.vm_config.code.specifier required` |
| 5 | DataSource `watched_directory` set | `wasm: config.vm_config.code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)` |
| 6 | AsyncDataSource `Remote` arm set | `wasm: config.vm_config.code: AsyncDataSource.Remote is not supported (envoy-go-strict)` |
| 7 | DataSource `Filename` empty | `wasm: config.vm_config.code: filename empty` |
| 8 | DataSource `Filename` read failure (ENOENT / EACCES / etc.) | `wasm: config.vm_config.code: read file %q: <inner>` (wrap) |
| 9 | DataSource `Filename` zero-byte contents | `wasm: config.vm_config.code: file %q is empty` |
| 10 | DataSource `InlineBytes` empty | `wasm: config.vm_config.code: inline_bytes empty` |
| 11 | `VmConfig.runtime` not in {`""`, `"envoy.wasm.runtime.wazero"`} | `wasm: config.vm_config.runtime %q is not supported; only "envoy.wasm.runtime.wazero" supported (envoy-go-strict)` |
| 12 | DataSource `InlineString` empty | `wasm: config.vm_config.code: inline_string empty` |
| 13 | DataSource `EnvironmentVariable` name empty | `wasm: config.vm_config.code: environment_variable name empty` |
| 14 | DataSource `EnvironmentVariable` unset | `wasm: config.vm_config.code: environment_variable %q not set` |
| 15 | DataSource `EnvironmentVariable` empty value | `wasm: config.vm_config.code: environment_variable %q is empty` |
| 16 | wasm-module ABI sentinel not `proxy_abi_version_0_2_1` (per AMEND-A6) | `wasm: config.vm_config.code: unsupported ABI version; only proxy_abi_version_0_2_1 supported (envoy-go-strict)` |
| 17 | wazero `CompileModule` parse/compile failure | `wasm: config.vm_config.code: compile: <inner>` (wrap) |
| 18 | per-route TPFC entry (NEW chosen at D-P6 at Task 16 — substring `"specifier"` arm; DEVIATED from anticipated arm 5 per the D-P6 closure) | (boot-reject differential fixture-0035 substring-asserted; full wording surfaces from arm-4-equivalent error path) |

**D-P5 closure at Task 9.** The 18-arm roster's byte-stable wording was pinned via `compiled_config_test.go::TestParseRejectConstants_ByteStable` at Task 9 — each arm's `parseReject*` const must be a string literal exactly matching the table row's wording. The 18 constants form the full PARSE-REJECT vocabulary at 25.1 phase-done; arms 6 + 11 are the 2 envoy-go-strict departures (Remote arm + runtime-name discriminator) — consolidated into departure record #3 below.

**D-P6 closure at Task 16.** Empirically scraped upstream Envoy v1.37.2 boot stderr for the boot-reject substring assertion: settled at substring `"specifier"` (DEVIATED from anticipated arm 5). The fixture-0035 single-arm boot-reject asserts the substring presence on both sides — upstream Envoy's PGV-mirror error for empty `vm_config.code.specifier` oneof produces error text containing `"specifier"`; envoy-go's arm-4 PARSE-REJECT wording (`"wasm: config.vm_config.code.specifier required"`) likewise contains `"specifier"`. Cross-side substring assertion verified GREEN at Task 16.

#### 24-hostcall + 13-callback bridge surface (per §5)

**Per-stream dispatch model:** at `DecodeHeaders`, the filter constructs a fresh `internal/wasm/VM` (cheap per-stream `*wazero.Runtime` per WEAK-default — Task 17 D-P4 R8 STANDS at `~61µs/stream` ns/op = 61000, well under 1ms threshold), registers the `ABICallbacks` bundle via `vm.RegisterABICallbacks(abiCallbacksImpl)`, calls `vm.Run(ctx, cfg.module, rootContextID=1)` which executes (a) wazero re-compile against vm.runtime (sub-ms hit via shared `wazero.CompilationCache`) + (b) `InstantiateModule` + (c) `_initialize` OR `_start` (mutually exclusive) + (c.5) `proxy_on_context_create(1, 0)` (root context seed per the canonical proxy-wasm host lifecycle; sandbox-gated) + (d) `proxy_on_vm_start(1, 0)` (sandbox-gated) + (e) `proxy_on_configure(1, 0)` (sandbox-gated). Then `vm.CallProxyOnRequestHeaders(ctx, streamCtxID, numHeaders, endOfStream) → ProxyAction`. PAUSE-return (=1) without prior `proxy_send_local_response` invocation is treated as CONTINUE per the v1.37.2 binding-gap `allow_on_headers_stop_iteration` field (per parent §5.7 — DEFERRED to 25.2). The encode-side surface mirrors decode for `proxy_on_response_headers`. `OnDestroy` calls `vm.Close()` releasing the wazero Runtime.

**24-hostcall surface (16 active `proxy_*` env-namespace + 8 active `wasi_*` shims; per §5.1 + §5.2):**

| Family | Hostcalls at 25.1 |
|---|---|
| Headers-bridge (7) | `proxy_get_header_map_pairs`, `proxy_set_header_map_pairs`, `proxy_add_header_map_value`, `proxy_replace_header_map_value`, `proxy_remove_header_map_value`, `proxy_get_header_map_value`, `proxy_get_header_map_size` |
| Local-response (1) | `proxy_send_local_response` |
| Log (2) | `proxy_log`, `proxy_get_log_level` |
| Property (2) | `proxy_get_property`, `proxy_set_property` (minimal 5-path tree at 25.1 — `request.protocol` / `request.path` / `request.method` / `request.host` / `response.code`) |
| Status (1) | `proxy_get_status` — REUSES the NEW `EncoderFilterCallbacks.ResponseStatus()` accessor per ADR-0196 (D-P3 closure at Task 11 — FIRST CO-CONSUMER of ADR-0196 after phase-23 admission_control's encode-classify discipline) |
| Time (1) | `proxy_get_current_time_nanoseconds` |
| Context (2) | `proxy_set_effective_context`, `proxy_done` |
| WASI custom 8-stub (per R4 + §5.2) | `fd_write` (fd=1 → `proxy_log INFO`, fd=2 → `proxy_log ERROR`), `fd_read` (always returns `WasiErrno::ENOTCAPABLE`=76), `fd_close`, `fd_fdstat_get`, `environ_get` / `environ_sizes_get` (return zeros), `clock_time_get`, `random_get`, `proc_exit` (traps via wazero) |

NOT wazero's built-in `imports/wasi_snapshot_preview1` package — its `fd_write` routes to host stdout/stderr rather than through `proxy_log` (envoy-go-strict "no stdout leak" discipline). The custom 8-stub lives at `internal/wasm/wasi.go` per R4 + ADR-0202 §Decision.

**23 deferred-to-25.2/25.3 hostcalls** registered as stub-Unimplemented at 25.1 (per parent §4.2 Option B): body+buffer family (4) + trailers family (3) + timer (1) + metrics (4) + shared-data (5) + httpCall (2) + foreign-function (1) + stream-control (2) + grpc family (5; 25.2). Each stub returns `WasmResult::Unimplemented` (=12); the integration error log fires per ADR-0204 + the `hostcall_denied` counter remains at zero (Unimplemented is NOT a denial — denial requires sandbox restriction).

**13 guest-export callbacks at 25.1 (host invokes these on the guest; per §5.3):**

| Family | Callbacks |
|---|---|
| Module-init / allocator (5; UNGATED per D-P2) | `_initialize`, `_start`, `main`, `malloc`, `proxy_on_memory_allocate` |
| Lifecycle (6; sandbox-gated per §3.3) | `proxy_on_context_create`, `proxy_on_vm_start`, `proxy_on_configure`, `proxy_on_done`, `proxy_on_log`, `proxy_on_delete` |
| HTTP-phase hooks (2; sandbox-gated) | `proxy_on_request_headers`, `proxy_on_response_headers` |

**D-P2 closure at Task 6.** Empirically scraped upstream `proxy-wasm-cpp-host:wasm.cc:298-302` (`Wasm::initializeFunctions` + the `_GET_FUNCTION` macro). The 5 module-init / allocator callbacks (`_initialize` / `_start` / `main` / `malloc` / `proxy_on_memory_allocate`) are UNGATED — they fire regardless of the sandbox `capability_restriction_config.allowed_capabilities` map contents. The 8 lifecycle / HTTP-phase callbacks ARE sandbox-gated via the `_GET_PROXY` macro (`wasm.cc:181-206`): when `vm.sandbox.IsAllowed(<key>)` returns false, the corresponding `ExportedFunction` lookup is skipped and the host treats the missing function as if the guest hadn't exported it (matches upstream's "nullptr the function pointer" discipline).

#### `proxy_send_local_response` byte-pin (per parent §11.6 + §5.1 hostcall 8)

For `proxy_send_local_response(403, msg_ptr, msg_size, body_ptr, body_size, addl_ptr, addl_size, -1)` from `proxy_on_request_headers`:

- `:status: 403` (wire status from the first hostcall arg)
- Body: bytes from `body_ptr..body_ptr+body_size` (envoy-go forwards verbatim — NO `content-length` auto-insertion at 25.1; the proxy-wasm hostcall envelope makes the guest responsible for assembling all headers the local-response carries).
- Additional headers: parsed from the `addl_ptr..addl_ptr+addl_size` pairs wire format per `internal/wasm/pairs.go` byte-faithful implementation (per R3); merged into the local-response header map.
- gRPC status: `-1` signals "non-gRPC local response"; the integer maps to a gRPC status only when ≥ 0.
- Subsequent guest return of `ProxyAction::PAUSE` (=1) triggers the REUSE-5 captured-local-response → `StopIteration` + `SendLocalReply` path. PAUSE without prior `proxy_send_local_response` invocation is treated as CONTINUE per the v1.37.2 binding-gap forward-pointer.

#### Per-stream `*wasm.VM` construction (WEAK-default per parent §4.2 + §13-R8) + per-module `*Module` compile cache (per parent §11.5)

`NewVM` constructs a fresh `*wazero.Runtime` per call — no pool at 25.1. Module re-compile against `vm.runtime` (REQUIRED because wazero v1.10.1's `CompiledModule` is bound to the engine of the runtime that compiled it per `wazero/cache.go:32-34`) is amortized via a SHARED `wazero.CompilationCache` plumbed through `NewVM(ctx, WithCompilationCache(cache.WazeroCompilationCache()))` — sub-ms cache hit on the second-and-later per-stream re-compile. Cache discipline: compiled modules (`*wazero.CompiledModule`) are wrapped by `*Module` (carries the sha256 cache key + the original wasm src for cross-runtime re-compile via the shared `CompilationCache`). Cache is owned by `*CompileCache` (filter-config-instance scope per D-P-PLAN-5, NOT global). API: `NewCompileCache(ctx) *CompileCache` + `CompileModule(ctx, src, cache) (*Module, error)`. Cache key is `sha256(src)` (32 bytes per content-addressable hashing).

**D-P4 R8 disposition — STANDS WEAK-default; ADR-0205 NOT consumed.** Per parent §13-R8 + 25.1 PLAN D-P-PLAN-10 codification: 25.1 IMPL Task 17 `BenchmarkPerStreamVM_Construction_Headers` measures the per-stream construction cost matching production `DecodeHeaders` step-by-step (NewVM + WithCompilationCache + vm.Run including re-compile + Instantiate + lifecycle dispatch + Close). The threshold gate per D-P4 fires ADR-0205 (the per-module wazero Runtime pool with pre-instantiated entries) only if `ns/op > 1_000_000` (= 1ms). **Observed:** `ns/op = 61000` (~61µs; 17566 iterations at 144212 B/op + 712 allocs/op — well under threshold). **Disposition: R8 STANDS WEAK-default — per-stream construction acceptable; ADR-0205 NOT consumed; carries forward to 25.2 BRAINSTORM** as the 25.2 IMPL escape-valve slot. 25.2 may re-evaluate against the body/buffer + advanced bridge surface (which adds more bridge methods + more per-stream allocation); ADR-0205 fires at 25.2 IMPL only if the advanced-bridge extension crosses the 1ms threshold.

#### Default-deny capability sandbox + envoy-go-strict departures

See **departure record #1** below for the default-deny capability sandbox + ADR-0204; **departure record #2** for ABI v0.1.0 + v0.2.0 PARSE-REJECT + AMEND-A6; **departure record #3** for the consolidated `AsyncDataSource.Remote` + runtime-name + 3 envoy-go-strict counters bundle.

#### envoy-go-strict departure record #1 — default-deny capability sandbox per AMEND-A5 + ADR-0204

Per phase-25 parent SPEC AMEND-A5 + ADR-0204 + the §11.4 D7 empirical scrape. Upstream Envoy v1.37.2's `proxy-wasm-cpp-host:include/proxy-wasm/wasm.h:103-106` (the `capabilityAllowed` gate function):

```cpp
bool capabilityAllowed(std::string capability_name) {
  return allowed_capabilities_.empty() ||
         allowed_capabilities_.find(capability_name) != allowed_capabilities_.end();
}
```

When `allowed_capabilities_` is empty (the default — also the result of unset `capability_restriction_config` OR its `allowed_capabilities` map empty), ALL hostcalls ALLOWED. envoy-go-strict INVERTS the empty-map semantic to DENY-ALL. Operators MUST explicitly enable each capability via `PluginConfig.capability_restriction_config.allowed_capabilities[<capability_name>] = SanitizationConfig{}`.

**Rationale:** WASM has a substantially larger and riskier hostcall surface than Lua (`proxy_call_foreign_function` for arbitrary host-side dispatch; `proxy_dispatch_http_call` for outbound network; `proxy_set_shared_data` for cross-stream state; `proxy_define_metric` for unbounded dynamic-stat namespace creation); upstream Envoy v1.37.2 marks its 3 sandbox runtimes (V8, WAMR, Wasmtime) as `status: alpha` + `security_posture: unknown` (per `source/extensions/extensions_metadata.yaml:1631-1635`) — the alpha-status posture is incompatible with envoy-go's safe-by-default discipline. Mirrors phase-22 lua's default-deny `SandboxConfig` posture (ADR-0188) at the analogous semantic + the project's security-first defaults pattern.

**Denial semantic** (byte-faithful from upstream `proxy_wasm_exports.h:217-226`): the hostcall returns `WasmResult::InternalFailure` (=10) to the guest; the host emits an integration error log via `slog.Error("Attempted call to restricted proxy-wasm capability: <name>")`; the `wasm.<plugin_name>.hostcall_denied` envoy-go-strict counter increments. **WASI denial errno** per D-P1 closure at Task 2: `WasiErrno::ENOTCAPABLE` = 76 (matches upstream byte-faithfully; envoy-go's WASI shim 8-stub returns this errno for any denied WASI key). **D-P1 evidence**: empirically scraped upstream `proxy-wasm-cpp-host:proxy_wasm_exports.h:232-249` — `WasiErrno::ENOTCAPABLE` is the canonical denied-capability errno for WASI hostcalls.

**`SanitizationConfig` accept-empty discipline** per AMEND-A1 §11.4. Upstream's `SanitizationConfig` proto is EMPTY (no fields) + upstream marks the per-capability sanitization layer "currently unimplemented and ignored, and so should be left empty" (`source/extensions/common/wasm/plugin.cc:14`). envoy-go matches upstream byte-faithfully — accept empty `SanitizationConfig{}` values; non-empty `SanitizationConfig` is accept-and-discard (mirrors phase-24's `override_option` INERT acceptance per ADR-0199 AMEND-4).

#### envoy-go-strict departure record #2 — ABI v0.1.0 + v0.2.0 PARSE-REJECT per AMEND-A6

Per phase-25 parent SPEC AMEND-A6 + the §11.5 D4 empirical scrape. Upstream Envoy v1.37.2 accepts ALL three proxy-wasm ABI versions (v0.1.0 + v0.2.0 + v0.2.1) and version-dispatches the registered hostcall + callback sets via `proxy-wasm-cpp-host:wasm.cc:286-293` (`registerCallbacks`) + `wasm.cc:298-302` (`getFunctions`); only `AbiVersion::Unknown` (absent sentinel export) is rejected at upstream. **envoy-go-strict targets v0.2.1 EXCLUSIVELY and PARSE-REJECTs v0.1.0 + v0.2.0 modules** at `internal/wasm/bytecode_util.go::GetAbiVersion`. This is an **envoy-go-strict-stricter** departure (NOT parity).

**Rationale:** v0.1.0 + v0.2.0 are evolutionary predecessors of v0.2.1; supporting all three multiplies the host-side dispatch surface 3× without adding operator-relevant functionality (operators authoring fresh wasm plugins target the latest spec). The narrower surface reduces audit cost + reduces the per-callback dispatch branch count.

**Detection point** at `internal/wasm/bytecode_util.go::GetAbiVersion`: scan the wasm module export section (type 7) for a function-kind export named `proxy_abi_version_0_2_1` (24 UTF-8 ASCII bytes). Byte-faithful reimplementation of `proxy-wasm-cpp-host:bytecode_util.cc:32-97`. Returns `AbiVersion_0_2_1` on match; any other ABI version (or absent sentinel) returns `AbiVersion_Unknown` + the calling `CompileModule` wraps with `ErrUnsupportedAbiVersion` per arm 16 wording (`"wasm: config.vm_config.code: unsupported ABI version; only proxy_abi_version_0_2_1 supported (envoy-go-strict)"`).

#### envoy-go-strict departure record #3 — consolidated bundle (Remote + runtime-name + 3 envoy-go-strict counters)

Consolidated departure bundle covering 5 sub-items per the AMEND-A2 + parent §13 + ADR-0203 §Decision unification:

1. **`AsyncDataSource.Remote` PARSE-REJECT** per arm 6 + parent §2.1. Upstream supports remote bytecode fetching via the runtime/RTDS surface. envoy-go-strict departure rationale: remote bytecode fetching introduces a runtime/RTDS dependency that lands at a future Runtime/hot-reload phase; the local-only 4-arm AsyncDataSource is sufficient for the 25.1 + 25.2 + 25.3 envelope. Byte-stable wording: `"wasm: config.vm_config.code: AsyncDataSource.Remote is not supported (envoy-go-strict)"`.

2. **Runtime-name discriminator PARSE-REJECT** per arm 11 + parent §2.3. Upstream Envoy v1.37.2 accepts 4 runtime discriminators (`envoy.wasm.runtime.{v8,wamr,wasmtime,null}`) at `source/extensions/common/wasm/wasm.cc:55-66`; envoy-go ships only wazero per AMEND-A1 + ADR-0202 + Q2 BRAINSTORM decision. The PARSE-REJECT wording uses the printf-`%q` formatter so the operator sees the violating discriminator quoted: `"wasm: config.vm_config.runtime %q is not supported; only \"envoy.wasm.runtime.wazero\" supported (envoy-go-strict)"`.

3-5. **3 envoy-go-strict counters bundle** per AMEND-A2 Group-C extension: `wasm.<plugin_name>.executions` + `wasm.<plugin_name>.hostcall_denied` + `wasm.<plugin_name>.envoy_go.failures`. **Upstream Envoy v1.37.2 emits ONLY the 2 upstream-parity Group-B counters** (`wasm.wazero.created` + `wasm.wazero.active` per `source/extensions/common/wasm/stats.h::WasmRuntimeStats`) plus the deferred-to-25.3 `vm_reload_*` Group-C triplet; the 3 envoy-go-strict additions surface operator-relevant signals NOT in the upstream observable surface (per-plugin invocation rate / sandbox-denial rate / failure-event rate). Operator-visibility rationale: knowing the per-plugin execution rate enables capacity planning + per-plugin alerting; the `hostcall_denied` counter pairs with the default-deny sandbox to surface misconfigured-capability dashboards; the `envoy_go.failures` counter provides observability into the envoy-go-side panic-wrapper-trip rate (a non-zero failure rate signals an operator script BUG that needs investigation).

The 3 counters are consolidated into one record (not 3 separate records per the 22.2 lua precedent) because they are jointly-introduced at the same SPEC commit + jointly-anchored at AMEND-A2 + ADR-0203 §Decision — they form a single stat-roster bundle, not 3 independent decisions.

#### envoy-go-strict departure record #4 — phase 25.2 9-counter consolidated bundle per AMEND-B3 (RAISES BRAINSTORM Q9 8 → 9)

Per 25.2 SPEC §7.1 + §9 + AMEND-B3 + Q9. **9 NEW envoy-go-strict counters land at 25.2**: `wasm.<plugin>.tick_invocations` + `http_call_dispatched` + `http_call_response` + `foreign_function_denied` + `body_buffer_cap_exceeded` + `http_call_dispatch_unknown_cluster` + `shared_data_cap_exceeded` + `dynamic_stats_cap_exceeded` + `http_call_response_after_close` (counter 14 — AMEND-B3 addition; expanded BRAINSTORM Q9's 8-counter tally to 9). Consolidated into a single departure record bundle per the same jointly-introduced + jointly-anchored discipline as 25.1 record #3 (NOT 9 separate records). **Upstream Envoy v1.37.2 does NOT emit any of these** — they are envoy-go-strict observability extensions for operator-visibility into tick rate / httpCall rate / foreign-function denial rate / cap-exceeded events / late-response-after-close defensive signal. Operator-visibility rationale: knowing tick dispatch rate enables capacity planning; the cap-exceeded counters surface guest-PAUSE-loop / shared-data-loop / dynamic-stats-namespace-explosion attacks; the `http_call_response_after_close` counter surfaces bugs in envoy-go's cancel-at-destruction discipline (non-zero pages an operator). **9 counters consolidated into ONE record** (not 9 separate) for the same jointly-introduced + jointly-anchored rationale + AMEND-B3 bundle anchoring. Bundled with the **4 envoy-go-strict-only `PluginConfig` config fields** that gate the cap-exceeded counters: `envoy_go_strict_body_buffer_cap_bytes` (default 16777216 = 16 MiB) + `envoy_go_strict_shared_data_value_cap_bytes` (default 1048576 = 1 MiB) + `envoy_go_strict_shared_data_max_entries` (default 1024) + `envoy_go_strict_dynamic_stats_max_entries` (default 1024). 4 envoy-go-strict-only `PluginConfig` extensions land at the envoy-go-internal `*compiledConfig` after parse, populated from a custom envoy-go protobuf extension via the `envoy_go_strict` struct (the existing `Configuration.Struct` Any wrapper).

#### envoy-go-strict departure record #5 — phase 25.2 body-buffer cap discipline per Q2

Per 25.2 SPEC §9 + Q2 + ADR-0208. **16 MiB envoy-go-strict default cap** on accumulated body buffer per `proxy_on_request_body` + `proxy_on_response_body` dispatch (the cap counts the accumulated total per the spec README — body_size grows monotonically when host buffers under PAUSE). Operator-configurable via `envoy_go_strict_body_buffer_cap_bytes` (default 16 MiB = 16777216 bytes; envoy-go-strict-only config field; PARSE-REJECT arm 19 on `=0`; arm 23 on `>1 GiB`). On cap exceeded: 413 Payload Too Large (decode side via `SendLocalReply(413, "wasm: body buffer cap exceeded", ...)`) OR response terminate (encode side); `wasm.<plugin>.body_buffer_cap_exceeded` counter + `wasm.<plugin>.envoy_go.failures` counter (per §2.25 scope extension) + integration error log.

**Rationale:** upstream Envoy v1.37.2 has NO in-filter cap on accumulated body buffer for wasm; relies on HCM-level + listener-level memory ceilings (`per_connection_buffer_limit_bytes`, etc.). envoy-go-strict adds defense-in-depth against PAUSE-loop guest patterns (a guest that repeatedly returns `PAUSE` from `proxy_on_request_body` without `proxy_send_local_response` would grow the host buffer unboundedly — caps short-circuit at 16 MiB). The 1 GiB ceiling (arm 23) provides a safety upper bound preventing operator-typo overflow into 0-cap behavior.

#### envoy-go-strict departure record #6 — phase 25.2 shared-data cap discipline + tick period 10ms floor (consolidated per Q5 + Q6)

Per 25.2 SPEC §9 + Q5 + Q6 + ADR-0205 + ADR-0208. Consolidated departure bundle covering 2 sub-items:

1. **Shared-data caps** (Q6). 1 MiB value cap (`envoy_go_strict_shared_data_value_cap_bytes`; default 1048576 = 1 MiB) + 1024-entry cap (`envoy_go_strict_shared_data_max_entries`; default 1024). Both operator-configurable; PARSE-REJECT arms 20 + 21 on `=0`. On cap exceeded: `WasmResult::InternalFailure` (=10) return-to-guest + `wasm.<plugin>.shared_data_cap_exceeded` counter + `wasm.<plugin>.envoy_go.failures` counter + integration error log. **Rationale:** upstream Envoy v1.37.2 has NO in-filter cap on shared-data namespace creation; relies on process memory exhaustion as the natural backstop. envoy-go-strict adds defense-in-depth — a guest looping `proxy_set_shared_data(unique_key, large_value, ...)` would consume host memory unboundedly without caps.

2. **Tick period 10ms floor** (Q5). `proxy_set_tick_period_milliseconds(period < 10)` is silently clamped to 10ms host-side at `internal/wasm/tick.go::effectivePeriod`. Period=0 cancels (canonical proxy-wasm behavior — unchanged from upstream). **Rationale:** prevents guest-driven CPU spin attacks (period=0 cancels; period=1ms would tight-loop the tick goroutine on a single core). Operators with legitimate sub-10ms timer use cases are NOT supported at envoy-go (defensive scope-narrowing per the safety-first defaults pattern; the 10ms floor matches upstream Envoy's `Watchdog.miss_timeout` 10ms default + the canonical CPU-watchdog granularity). The floor is a compile-time constant (NOT a config field) per 25.2 SPEC §2.16 + §3.1.

#### envoy-go-strict departure record #7 — phase 25.2 foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace clarification (consolidated per AMEND-A9 + Q9 + AMEND-B2)

Per 25.2 SPEC §9 + AMEND-A9 + Q9 + AMEND-B2 + ADR-0206 + ADR-0208. Consolidated departure bundle covering 3 sub-items:

1. **Foreign-function 0-vs-10 default registry** (AMEND-A9). Upstream Envoy v1.37.2 registers 10 default foreign functions at `source/extensions/common/wasm/foreign.cc` (`verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`); **envoy-go registers ZERO by default**. Operators MUST explicitly enable the `proxy_call_foreign_function` capability AND register specific foreign functions via `wasm.RegisterForeignFunction(name string, fn wasm.ForeignFunctionFn)` at boot (process-global `wasm.DefaultForeignFunctionRegistry`). Unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream + increment `wasm.<plugin>.foreign_function_denied` counter. **Rationale:** the 10 upstream defaults include cryptographic primitives (`verify_signature` / `sign`) + arbitrary CEL evaluation (`expr_*`) + envoy-internal mutation primitives (`set_envoy_filter_state` / `clear_route_cache`) — each is an attack-surface increment vs. the safe-by-default discipline. Operators opt-in per use case + per function.

2. **Dynamic-stats 1024-entry cap** (Q9). `proxy_define_metric` capped at 1024 entries per plugin (default; operator-configurable via `envoy_go_strict_dynamic_stats_max_entries` envoy-go-strict-only config field; PARSE-REJECT arm 22 on `=0`). Cap-exceeded → `wasm.<plugin>.dynamic_stats_cap_exceeded` counter + `WasmResult::InternalFailure` (=10) return-to-guest. **Rationale:** upstream Envoy v1.37.2 has NO in-filter cap on dynamic-stats namespace creation; relies on stats-sink memory exhaustion as the natural backstop. envoy-go-strict adds defense-in-depth — a guest looping `proxy_define_metric(unique_name)` would create a stats-namespace explosion without caps.

3. **Dynamic-stats namespace `wasmcustom.<custom_name>`** (AMEND-B2). The namespace shape is **NO plugin prefix in the namespace** (upstream byte-faithful per BRAINSTORM Q9 REFINEMENT); per-plugin isolation via per-plugin `*dynamic.Registry` SCOPE — each `*compiledConfig` constructs its own Registry rooted at the per-plugin stat scope (`stats.RootScope.Subscope("wasm").Subscope(pluginName)`); the Registry produces stat names `wasmcustom.<custom_name>` under that parent scope. From the operator's perspective, the admin `/stats` endpoint enumerates these as `wasm.<plugin_name>.wasmcustom.<custom_name>` (the parent scope's name + the wasmcustom child) — but the in-wire stat name (from the proxy-wasm wire perspective) is `wasmcustom.<custom_name>` byte-faithful to upstream. NEW `internal/stats/dynamic/` infrastructure subpackage anchors the Registry primitive per ADR-0208 + AMEND-B2. **NOT counted in the static stat name total** at 128 (the 9 25.2 envoy-go-strict counters tallied at §7.1; the dynamic family is operator-extensible).

**Note: structural-divergence-from-§9-family-pattern row (NOT a departure from upstream).** The wasm filter's stat surface DROPS the HCM-injected `stats_prefix` segment that every prior §9 family-row's stat surface incorporates (upstream `source/extensions/filters/http/wasm/config.h:51-53` drops the HCM stats_prefix; envoy-go matches). This is upstream-parity preservation, NOT envoy-go-strict — recorded as a special-case row at the `## Stat-name mapping` extension above rather than as a departure record. Mirrors phase-15 bandwidth_limit's non-HCM-rooted-shape divergence-from-family-pattern record (a structural note, not a departure).

#### Phase 25.2 EXTENSION — full advanced-bridge surface ACTIVATED (per ADR-0205 + ADR-0206 + ADR-0207 + ADR-0208 + AMEND-B1..B5)

Phase 25.2 ships the **full advanced-bridge surface delta** for `envoy.filters.http.wasm` (body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full property bridge — all 25.2 callbacks + 14 NEW env-namespace hostcalls). This is the EIGHTEENTH §9 family-row's SECOND of 3 sub-phases (parent row 25 STAYS `in-progress` until 25.3 phase-done). **4 anchored ADRs at phase 25.2**:

- **ADR-0205 Root VM lifecycle evolution (per Q3)** — ONE long-lived `*RootVM` per `*compiledConfig` (upstream-byte-faithful per cpp-host `Wasm`/`Plugin` model). Per-stream contexts as CHILDREN sharing the wazero Runtime + compiled `*Module` + foreign-function registry view + shared-data + httpCall routing state at root. Per-`*RootVM` tick goroutine + 10ms envoy-go-strict period floor + Clock seam FIRST co-consumer beyond phase-21 RATIFIES ADR-0186. **25.1 per-stream `*VM` RETIRED** per D-P-PLAN-6 (the 25.1 `internal/wasm/vm.go` file deleted at Task 1; `internal/wasm/root_vm.go` + `internal/wasm/stream_context.go` materialize). Per-stream Module instantiation pattern: **WEAK-default fresh-per-stream STANDS** per the Task 22 R8 benchmark gate (`BenchmarkPerStreamModule_Instantiation` measured ~98 ns/op << 1ms threshold; ADR-0209 escape-valve STAYS UNCONSUMED + carries forward to 25.3 IMPL escape-valve slot).
- **ADR-0206 25.2 ABI extensions** — 14 NEW env-namespace hostcalls + 7 NEW guest-export callbacks + 21 NEW capability keys gate-at-`registerCallback` time per AMEND-B5 + buffer-clamp wire-contract per AMEND-B1 + metric signedness pin per AMEND-B2 (signed-int64 deltas; unsigned-uint64 record values) + `internal/wasm/foreign.go` ForeignFunctionRegistry with EMPTY default registry per AMEND-A9 + full ~70-path proxy_get_property roster per AMEND-B4 + mutex-per-RootVM foreign-function concurrency model per D-25.2-P3 + D-P-PLAN-9.
- **ADR-0207 NEW `internal/filterstate/` framework primitive** (SECOND occurrence of EXTRACT-NOW-on-second-consumer after phase-22.1+22.2's `internal/lua/`+`internal/dynamicmetadata/`) — `*Bucket` + `FilterStateObject` interface + `StateType` enum + read-only-vs-mutable sync semantics. **Consumer #1**: phase-22.2 `internal/filter/http/lua/filterstate.go` MIGRATES non-breaking at Task 10 (`:filterState()` Lua surface UNCHANGED; existing phase-22.2 lua filterstate tests GREEN without modification). **Consumer #2**: phase-25.2 wasm filter `filter_state.*` + `upstream_filter_state.*` property roots per AMEND-B4. EXPLICIT API-REVISION ALLOWANCE clause for consumer #3+ (rbac filter-state read; ext_authz filter-state inject; ext_proc filter-state pass-through; new filter families).
- **ADR-0208 NEW `internal/filter/http/wasm/` 25.2 package extensions** — 9 envoy-go-strict counters per §7.1 + AMEND-B3 (counter 14 `http_call_response_after_close` per AMEND-B3 — defensive observability for the cancel-at-destruction race) + 4 envoy-go-strict-only `PluginConfig` config fields per Qs 2/6/9 + dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2 via NEW `internal/stats/dynamic/` infrastructure (per-plugin `*dynamic.Registry` SCOPE discipline) + mixed-mode fixture-0036 + subject-only boot-reject fixture-0037 + 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` per §8.4 + R-25.2-12.

**ADR-0202 §Consequences one-line in-place AMEND acknowledgment** (per §10.2 + ADR-0044 in-place edit discipline; no new ADR number consumed) — phase 25.2 introduces consumer-#1-internal-scope API evolution (root VM lifecycle per ADR-0205; foreign-function registration per ADR-0206 + AMEND-A9; per-stream Module instantiation pattern); the EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (broader §9 WASM host family) remains SCOPED to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent.

**6 NEW envoy-go-strict departure records at 25.2** consolidated bundle per §13.4 edits #3-#6 + §9: (1) 9-counter consolidated bundle per AMEND-B3 (RAISES BRAINSTORM Q9 8 → 9 — adds `http_call_response_after_close`); (2) body-buffer cap discipline per Q2 (16 MiB default; 413-on-exceed); (3) shared-data cap discipline + tick period 10ms floor consolidated per Q5+Q6; (4) foreign-function 0-vs-10 default registry + dynamic-stats cap + namespace clarification per AMEND-A9+Q9+AMEND-B2. Cumulative envoy-go-strict departure record count post-25.2: ~27 (21 inherited from 25.1 + 6 NEW at 25.2 consolidated bundle).

##### 25.2 hostcall surface delta — 14 NEW env-namespace + 7 NEW guest-export callbacks (per §5)

**14 NEW env-namespace hostcalls at 25.2** (REGISTRATION gated by the per-capability sandbox keys per AMEND-B5; module instantiation fails if the guest auto-imports a hostcall whose capability is denied):

| Family | Hostcalls activated at 25.2 |
|---|---|
| Body + buffer (3) | `proxy_get_buffer_bytes`, `proxy_set_buffer_bytes`, `proxy_get_buffer_status` |
| Stream-control (2) | `proxy_continue_stream`, `proxy_close_stream` |
| Timer (1) | `proxy_set_tick_period_milliseconds` |
| Metrics (4) | `proxy_define_metric`, `proxy_increment_metric`, `proxy_record_metric`, `proxy_get_metric` |
| Shared-data (2) | `proxy_set_shared_data`, `proxy_get_shared_data` |
| HTTP-call (1) | `proxy_http_call` (dispatch; routes to `proxy_on_http_call_response` callback) |
| Foreign-function (1) | `proxy_call_foreign_function` |

**7 NEW guest-export callbacks at 25.2** (sandbox-gated per AMEND-B5 gate-at-`registerCallback` discipline):

| Family | Callbacks |
|---|---|
| Body (2) | `proxy_on_request_body`, `proxy_on_response_body` |
| Trailers (2) | `proxy_on_request_trailers`, `proxy_on_response_trailers` |
| Timer (1) | `proxy_on_tick` |
| HTTP-call response (1) | `proxy_on_http_call_response` |
| Foreign-function (1) | `proxy_on_foreign_function` |

**Cumulative surface at 25.2 phase-done: 30 active env-namespace hostcalls + 8 active WASI shims (UNCHANGED) + 20 active guest-export callbacks** (5 module-init/allocator UNGATED + 6 lifecycle sandbox-gated + 2 HTTP-phase + 7 NEW at 25.2). The 25.1's "23 deferred-to-25.2/25.3 hostcalls registered as stub-Unimplemented" tally is RETIRED at 25.2: the 14 NEW above land as full implementations; the remaining 9 deferred-to-25.3 (5 gRPC family + 4 shared-queue family) STAY stub-Unimplemented.

##### Body buffer dispatch — accumulated-size discipline + 16 MiB envoy-go-strict cap per Q2

`proxy_on_request_body(streamCtxID, body_size, end_of_stream)` + `proxy_on_response_body(streamCtxID, body_size, end_of_stream)` fire per-chunk; `body_size` is the **accumulated total available** (NOT just-new-chunk delta), grows monotonically when host buffers under PAUSE per spec README. The host-side accumulator caps at the `envoy_go_strict_body_buffer_cap_bytes` envoy-go-strict-only config (default 16 MiB = 16777216 bytes; PARSE-REJECT arm 19 on `=0`; arm 23 on `>1 GiB`). Cap-exceeded path (decode side): the host sends `SendLocalReply(413, "wasm: body buffer cap exceeded", ...)` + closes the stream + increments `wasm.<plugin>.body_buffer_cap_exceeded` + `wasm.<plugin>.envoy_go.failures` counters per §2.25 scope extension. See **departure record #4 below**.

##### Trailers dispatch — REUSES 25.1 header-map family per §5.3

`proxy_on_request_trailers(streamCtxID, num_trailers)` + `proxy_on_response_trailers(streamCtxID, num_trailers)` REUSE the 25.1 header-map hostcall family (`proxy_get_header_map_*` / `proxy_set_header_map_*` / etc.) with `WasmHeaderMapType` values `1` (HttpRequestTrailers) + `3` (HttpResponseTrailers) ACTIVATED at 25.2 (the 25.1 header-map hostcalls dispatched only against values 0/2 — request/response headers). Zero new hostcalls; only the value-discriminator surface widens.

##### Timer dispatch — per-RootVM tick goroutine + 10ms envoy-go-strict floor per Q5

`proxy_set_tick_period_milliseconds(period_ms)` (hostcall) + `proxy_on_tick(rootCtxID)` (callback). One tick goroutine PER `*RootVM` (NOT per-stream — root-context-scoped) started at `Configure` time IF the guest set a non-zero period; period=0 cancels. The host clamps `period_ms < 10` to `10` silently (Q5 envoy-go-strict floor; prevents period=0 → hot-loop guest attacks; prevents 1ms tight-loop). Tick fires on the root context (`proxy_set_effective_context(rootCtxID)` automatically). Clock seam injection via `wasm.WithRootClock(clk clock.Clock)` — FIRST co-consumer of phase-21 `internal/clock/` ADR-0186 beyond phase-21 itself (RATIFIES the EXTRACT-NOW-on-second-consumer disposition for ADR-0186 at second-consumer scope). See **departure record #5 below**.

##### Metric hostcall dispatch — per-plugin Registry scope + signed-int64 delta per AMEND-B2

`proxy_define_metric(metric_type, name_ptr, name_size, *metric_id_out) -> WasmResult` registers a new dynamic metric on the per-plugin `*dynamic.Registry` (scope = `wasm.<plugin>.wasmcustom.<custom_name>` at admin /stats; the wire name from the proxy-wasm perspective is `wasmcustom.<custom_name>` per AMEND-B2). `MetricType`: Counter=0; Gauge=1; Histogram=2. `proxy_increment_metric(metric_id, offset_i64)` (SIGNED delta per AMEND-B2; negative offsets supported for Gauge); `proxy_record_metric(metric_id, value_u64)` (unsigned absolute value); `proxy_get_metric(metric_id, *value_out)`. Cap: 1024 entries per plugin (default; configurable via `envoy_go_strict_dynamic_stats_max_entries` envoy-go-strict-only config); cap-exceeded → `WasmResult::InternalFailure` + `wasm.<plugin>.dynamic_stats_cap_exceeded` counter increment. Per-plugin Registry SCOPE discipline: each `*compiledConfig` constructs its own `*dynamic.Registry` rooted at the per-plugin stat scope (`stats.RootScope.Subscope("wasm").Subscope(pluginName)`); cross-plugin metric isolation enforced at the scope-tree level. See **departure record #6 below**.

##### Shared-data hostcall dispatch — CAS + envoy-go-strict caps per Q6

`proxy_set_shared_data(key_ptr, key_size, value_ptr, value_size, cas_u32) -> WasmResult` writes with CAS check: `cas=0` always writes; `cas>0` writes only if the stored CAS matches (returns `WasmResult::CasMismatch` (=8) on conflict). `proxy_get_shared_data(key_ptr, key_size, *value_data_out, *value_size_out, *cas_out) -> WasmResult`. Per-`*RootVM` scope (NOT cross-plugin — multi-plugin cross-VM shared-data scoping DEFERRED to 25.3). envoy-go-strict caps: 1 MiB value cap (`envoy_go_strict_shared_data_value_cap_bytes`; PARSE-REJECT arm 20 on `=0`); 1024-entry cap (`envoy_go_strict_shared_data_max_entries`; PARSE-REJECT arm 21 on `=0`). Cap-exceeded → `WasmResult::InternalFailure` + `wasm.<plugin>.shared_data_cap_exceeded` counter + `wasm.<plugin>.envoy_go.failures` counter. See **departure record #5 below**.

##### proxy_http_call dispatch — cancel-at-destruction + late-response defensive guard per AMEND-B3

`proxy_http_call(cluster_name_ptr, cluster_name_size, headers_ptr, headers_size, body_ptr, body_size, trailers_ptr, trailers_size, timeout_ms, *call_token_out) -> WasmResult` dispatches an outbound HTTP request to the named upstream cluster via the per-`*RootVM` HTTPDispatcher (RE-CONSUMES phase-20 `internal/httpclient/` per ADR-0177 third-or-later co-consumer + ADR-0207 §3.4 MIGRATES — `internal/cluster/` cluster-lookup primitive RE-USED). Returns `WasmResult::Ok` (=0) + populates `call_token`; on unknown cluster returns `WasmResult::BadArgument` (=2) + increments `wasm.<plugin>.http_call_dispatch_unknown_cluster` counter; in-flight requests are CANCELLED at the originating `StreamContext.Close` (`OnDestroy`); defensive `wasm.<plugin>.http_call_response_after_close` counter (AMEND-B3 — RAISES BRAINSTORM Q9 8 → 9) increments if a stray response slips past the cancel guard (operationally near-zero in healthy operation; non-zero pages an operator). `proxy_on_http_call_response(streamCtxID, call_token, num_headers, body_size, num_trailers)` callback delivers the response to the originating stream context's frame. Concurrent dispatch from N concurrent streams to the same RootVM uses a mutex-per-RootVM serialization model per D-25.2-P3 closure at the PLAN session (D-P-PLAN-9).

##### Foreign-function dispatch — EMPTY default registry per AMEND-A9 + mutex-per-RootVM concurrency model

`proxy_call_foreign_function(name_ptr, name_size, args_ptr, args_size, *retptr_out, *retsize_out) -> WasmResult`. The host-side registry is **EMPTY by default** at envoy-go-strict per AMEND-A9 + ADR-0206 §Decision (upstream Envoy v1.37.2 registers 10 default foreign functions — `verify_signature`, `sign`, `compress`, `uncompress`, `set_envoy_filter_state`, `clear_route_cache`, `expr_create`, `expr_evaluate`, `expr_delete`, `declare_property`; envoy-go registers ZERO). Operators register host-side foreign functions via `wasm.RegisterForeignFunction(name string, fn wasm.ForeignFunctionFn)` at boot (process-global `wasm.DefaultForeignFunctionRegistry`); per-RootVM override via `wasm.WithRootForeignRegistry(reg)` for multi-tenant isolation. Unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream + increment `wasm.<plugin>.foreign_function_denied` counter. Concurrent dispatch serializes through a mutex on the per-RootVM registry view (per D-25.2-P3 closure at PLAN session; verified at IMPL Task 7 with concurrent N=100 dispatch + no cross-stream argument leak). See **departure record #6 below**.

##### Full proxy_get_property surface — ~70-path roster + NUL-delimited path serialization per AMEND-B4

`proxy_get_property(path_ptr, path_size, *value_ptr_out, *value_size_out) -> WasmResult` resolves a NUL-delimited byte-segmented path against the per-stream property tree. Path format: `request\0headers\0x-foo` (byte segments delimited by `\0` octet); empty segments tolerated; trailing NUL tolerated; non-NUL separator returns `WasmResult::NotFound`. Roster at 25.2 (~70 sub-paths consolidated under 10 dispatched roots + 4 direct tokens per AMEND-B4 D-25.2-4 SUBSTANTIVE REFINEMENT vs the BRAINSTORM Q7 hypothesized ~25 paths):

| Root | Coverage |
|---|---|
| `request.*` (~12 sub-paths) | method, path, host, scheme, headers.<name>, body, total_size, time, protocol, query, referer, useragent, id, duration, size |
| `response.*` (~10 sub-paths) | code, code_details, flags, total_size, headers.<name>, trailers.<name>, body, grpc_status |
| `connection.*` (~6 sub-paths) | id, mtls, requested_server_name, subject_local_certificate, subject_peer_certificate, sha256_peer_certificate_digest, dns_san_local_certificate, dns_san_peer_certificate, uri_san_local_certificate, uri_san_peer_certificate, termination_details, tls.version |
| `upstream.*` (~6 sub-paths) | address, port, cluster, transport_failure_reason, tls.version |
| `xds.*` (~8 sub-paths; CONSOLIDATES listener + route + cluster per AMEND-B4) | listener_metadata, route_metadata, cluster_metadata, listener_name, route_name, virtual_host_name, cluster_name |
| `source.*` (4) | address, port |
| `destination.*` (4) | address, port |
| `node.*` (~6) | id, cluster, metadata, locality.region, locality.zone, locality.sub_zone |
| `cluster_name`, `route_name`, `listener_direction`, `plugin_name` (4 direct tokens) | direct shortcut tokens; UNCONFIGURED returns NotFound |
| `filter_state.*` + `upstream_filter_state.*` (DISTINCT roots per AMEND-B4) | per-stream filter-state dispatch via NEW `internal/filterstate/` framework primitive (ADR-0207 consumer #2) |

Co-consumed primitives at the property tree: ADR-0144 (DownstreamPrincipal — `connection.subject_peer_certificate` + `connection.uri_san_peer_certificate` family); ADR-0177 (httpclient — `upstream.*` family); ADR-0190 (dynamicmetadata — `xds.*` family); NEW ADR-0207 (filterstate — `filter_state.*` + `upstream_filter_state.*` family). Path serialization is NUL-delimited (NOT envoy-go-strict — upstream-parity preservation; recorded as a wire-shape note, not a departure record).

##### `wasmcustom.<custom_name>` dynamic-stats namespace per AMEND-B2 + per-plugin Registry SCOPE

Plugin-defined dynamic stats register under the wire-level namespace `wasmcustom.<custom_name>` (NO plugin prefix in the namespace — upstream byte-faithful). Per-plugin isolation enforced via per-plugin `*dynamic.Registry` SCOPE — each `*compiledConfig` constructs its own `*dynamic.Registry` rooted at `stats.RootScope.Subscope("wasm").Subscope(pluginName)`; the Registry produces stat names `wasmcustom.<custom_name>` under that parent scope. From the operator's perspective, the admin `/stats` endpoint enumerates these as `wasm.<plugin_name>.wasmcustom.<custom_name>` (the parent scope's name + the wasmcustom child) — but the in-wire stat name (from the proxy-wasm wire perspective) is `wasmcustom.<custom_name>` byte-faithful to upstream. Operator-extensible at runtime via `proxy_define_metric` (capped at 1024 entries envoy-go-strict per `envoy_go_strict_dynamic_stats_max_entries`; cap-exceeded → counter 13 + `WasmResult::InternalFailure`).

##### Buffer-bounds CLAMP wire-contract per AMEND-B1 (wire-shape note; NOT a departure record)

`proxy_get_buffer_bytes(buffer_id, start, max_size, *return_data_ptr_out, *return_size_out)` clamps on overflow byte-faithful to cpp-host reference implementation: `if start > buffer.size → return Ok with length=0`; `if start+max_size > buffer.size → return Ok with truncated length (= buffer.size - start)`; only `start+max_size i32-overflow` returns `BadArgument`. The v0.2.1 spec README text saying `BAD_ARGUMENT` on overflow is REFINED here per the cpp-host empirical scrape at D-25.2-1 §11.1. NOT an envoy-go-strict departure (upstream-parity preservation against the reference cpp-host); recorded as a wire-shape note at this 25.2 EXTENSION subsection.

##### 25.2 PARSE-REJECT roster extension — 6 NEW arms per §6.2

Per AMEND-B5 + 25.2 SPEC §6.2. 6 NEW arms 19-24 extend the 25.1 18-arm roster:

| Arm | Trigger | Wording (byte-exact) |
|---|---|---|
| 19 | `envoy_go_strict.body_buffer_cap_bytes` = 0 | `wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)` |
| 20 | `envoy_go_strict.shared_data_value_cap_bytes` = 0 | `wasm: config.envoy_go_strict_shared_data_value_cap_bytes must be > 0 (envoy-go-strict)` |
| 21 | `envoy_go_strict.shared_data_max_entries` = 0 | `wasm: config.envoy_go_strict_shared_data_max_entries must be > 0 (envoy-go-strict)` |
| 22 | `envoy_go_strict.dynamic_stats_max_entries` = 0 | `wasm: config.envoy_go_strict_dynamic_stats_max_entries must be > 0 (envoy-go-strict)` |
| 23 | `envoy_go_strict.body_buffer_cap_bytes` > 1 GiB | `wasm: config.envoy_go_strict_body_buffer_cap_bytes %d exceeds 1 GiB ceiling (envoy-go-strict)` |
| 24 | (reserve — placeholder for IMPL Task NN) | (reserve; settles at IMPL if surfaces) |

**D-25.2-P1 closure at Task 21 first-action** — boot-reject fixture-0037 single-arm chose **arm 19 `envoy_go_strict_body_buffer_cap_bytes` zero** with distinctive substring `"envoy_go_strict_body_buffer_cap_bytes"`. Runner branch shape settled at IMPL Task 21: `BootRejectFixture` EXTENDED with `subjectOnly: true` flag (recommended-of-2 candidates settled at PLAN; subject-only because reference Envoy v1.37.2 silently drops the unknown envoy-go-strict-only field per its protobuf parser). **D-25.2-P5 closure at this Task 22 BEHAVIOR_CONTRACT.md bundle landing** — the 6 arms' byte-stable wording pinned at `compiled_config_test.go::TestParseRejectConstants_ByteStable` table-driven assertion (extended from the 25.1 18-arm roster to the 25.2 24-arm roster).

##### Phase 25.2 PARSE-REJECT roster extension — D-25.2-P2 + R8 disposition

**D-25.2-P2 + R8 disposition: STANDS WEAK-default fresh-per-stream Module instantiation** — `BenchmarkPerStreamModule_Instantiation` measured ~98 ns/op (well under 1ms threshold per D-P-PLAN-11). ADR-0209 escape-valve STAYS UNCONSUMED at this 25.2 IMPL Task 22 atomic landing + carries forward to 25.3 IMPL escape-valve slot per the R8 signaling protocol. The root-VM model RETIRES the 25.1 per-stream Runtime construction (`ns/op = 61000` at 25.1) — per-stream cost at 25.2 is bookkeeping + `proxy_on_context_create` dispatch on a shared `*RootVM.runtime` + shared compiled `*Module`.

##### Phase 25.2 cumulative roster summary

- **30 active env-namespace hostcalls** at 25.2 (16 active at 25.1 + 14 NEW at 25.2; 9 stubs remain — gRPC + shared-queue families deferred to 25.3).
- **8 active WASI shims** UNCHANGED at 25.2.
- **20 active guest-export callbacks** at 25.2 (13 at 25.1 + 7 NEW at 25.2).
- **24-arm PARSE-REJECT roster** at 25.2 (18 at 25.1 + 6 NEW arms 19-24 at 25.2).
- **58 cumulative capability keys** at 25.2 (37 at 25.1 + 21 NEW at 25.2 per AMEND-B5).
- **9 envoy-go-strict counters added at 25.2** (5 at 25.1 + 9 NEW at 25.2; project stat surface 119 → 128) plus the OPEN-ENDED `wasmcustom.<custom_name>` dynamic-stats family (NOT counted in static total).
- **4 envoy-go-strict-only `PluginConfig` config fields at 25.2** — `envoy_go_strict_body_buffer_cap_bytes` (default 16777216) + `envoy_go_strict_shared_data_value_cap_bytes` (default 1048576) + `envoy_go_strict_shared_data_max_entries` (default 1024) + `envoy_go_strict_dynamic_stats_max_entries` (default 1024).
- **39 differential fixture directories** (37 at 25.1 + 2 NEW at 25.2: fixture-0036 mixed-mode + fixture-0037 subject-only boot-reject).
- **35 project-wide fuzzers** (34 at 25.1 + 1 NEW at 25.2: `FuzzWasmHostcallEnvelope` per §8.4 + R-25.2-12).

##### Cross-references to NEW ADRs

ADR-0205 §Decision body anchors the root-VM lifecycle evolution. ADR-0206 §Decision body anchors the 25.2 ABI extensions + the buffer-clamp + metric signedness + foreign-function dispatch surface. ADR-0207 §Decision body anchors the NEW `internal/filterstate/` framework primitive + phase-22.2 lua MIGRATES non-breaking + EXPLICIT API-REVISION ALLOWANCE for consumer #3+. ADR-0208 §Decision body anchors the `internal/filter/http/wasm/` package extensions + the 9 envoy-go-strict counters + the 4 envoy-go-strict-only config fields + the `wasmcustom.<custom_name>` dynamic-stats namespace via NEW `internal/stats/dynamic/` infrastructure + the mixed-mode fixture-0036 + the subject-only boot-reject fixture-0037 + the 35th project-wide fuzzer.

ADR-0202 §Consequences AMEND acknowledgment (one-line in-place AMEND per §10.2): no new ADR number consumed; the EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 STAYS scoped to consumer #2.

#### Phase 25.3 EXTENSION — per-route + multi-plugin VM-sharing + FAIL_RELOAD + env_vars ACTIVATED (per ADR-0210 + ADR-0211 + ADR-0212)

Phase 25.3 is the **per-route + multi-plugin + reload + env_vars + conformance-harness THIRD-and-FINAL sub-phase** of `envoy.filters.http.wasm` — the §9 HTTP-filters family CLOSES at phase 25.3 phase-done. **3 NEW ADR bodies** land (ADR-0210 per-route REUSE-by-absence; ADR-0211 multi-plugin VM-sharing + reload + env_vars BUNDLED; ADR-0212 conformance harness seed) + 2 in-place amendments (ADR-0205 §Consequences AMEND `*RootVM per *compiledConfig` → per-`(vm_id, vm_configuration, code)` shared; ADR-0186 §Consequences RATIFIES the phase-21 clock consumer-real migration onto the unified `internal/clock` superset).

**Per-route wholesale-override semantics (subject-only; no `WasmPerRoute`) per ADR-0210.** A route's `typed_per_filter_config` carries a wholesale `envoy.extensions.filters.http.wasm.v3.Wasm` message (the SAME type as the listener-level config); per-route resolution is a WHOLESALE REPLACEMENT (`resolvePerRoute`: per-route override → listener-level → no-op; the phase-13/14/15 3-tier resolver REUSE). NO `WasmPerRoute` type; ADR-0125 STAYS at 10 canonicals. **Empirical finding: reference Envoy v1.37.2 has NO per-route wasm support** (`WasmFilterConfig` overrides only `createFilterFactoryFromProto*`, NOT `createRouteSpecificFilterConfig`; boot-rejects any per-route wasm config) — per-route wasm is therefore an **envoy-go capability surfaced subject-only** in the differential (fixture 0038 `perroute_override_applies` asserts via `wasm.plugin_perroute_override.executions`, the reference bootstrap carrying no per-route TPFC).

**Multi-plugin `vm_id`-shared VM + raw-`vm_id` shared-data per ADR-0211.** A process-global `*Registry` (`internal/wasm/registry.go`) keyed by `makeVMKey = Sha256(vm_id‖vm_configuration‖code)` + refcount shares ONE `*RootVM` across all `PluginConfig`s with matching identity; each gets a distinct per-stream child context against the shared instance. **Cross-plugin shared-data is SHARED at the broader raw-`vm_id` scope** (a separate `sharedDataByVmID` store keyed by the raw user `vm_id`, mirroring cpp-host `SharedData::data_`) — distinct VM instances under the same `vm_id` observe ONE shared-data namespace. envoy-go COLLAPSES cpp-host's two-layer (process-global + thread-local) into one process-global registry.

**FAIL_RELOAD reload state machine per ADR-0211 (RuntimeError-gated).** `FailurePolicy {UNSPECIFIED=0, FAIL_RELOAD=1, FAIL_CLOSED=2, FAIL_OPEN=3}`; UNSPECIFIED → FAIL_CLOSED (503), NOT bypass. A per-`*RootVM` `{Running, Reloading, Failed}` machine, request-driven + backoff-rate-limited, gated to guest `FailState::RuntimeError` only. **Backoff honors `base_interval` only** (`max_interval` is DEAD in the wasm path upstream) with an envoy-go-strict floor `base_interval = max(operator, 100ms)`. **Critical ordering: `ReloadDispatch` runs BEFORE `initStreamContext`/`proxy_on_context_create`** in `decode_headers.go` (the BUG-4 fix) so a Failed FAIL_RELOAD VM reinstantiates a fresh un-poisoned instance before context-create is attempted — the only sequence under which a guest-trap-poisoned instance recovers; the whole reload-then-context-create dispatch is serialized under `*RootVM.dispatchMu` (D-25.3-P3). `fail_open` (deprecated field 5) ⊕ `failure_policy` (field 7) are mutually exclusive (both-set → PARSE-REJECT); `fail_open=true → FAIL_OPEN`. The **`vm_reload` triplet** (`vm_reload_success` / `vm_reload_runtime_failure` / `vm_reload_backoff`) is the Group-C upstream-parity stat surface (see the stat-table rows above): a guest trap within the backoff window → `vm_reload_backoff`; a reinstantiate-recover past the window → `vm_reload_success`; a failed reload attempt → `vm_reload_runtime_failure`.

**`env_vars` activation per ADR-0211 (collision-REJECT + cap + WASI environ feed).** `AssembleEnvVars` merges `host_env_keys` (host-process lookup; absent keys silently skipped) + `key_values`. **Collisions across the two fields PARSE-REJECT at config-load** (byte-faithful to upstream — NO override semantic). The assembled map feeds the wazero WASI `environ_get` + `environ_sizes_get` shims as `KEY=VALUE\0` entries (the shims returned zeros at 25.2 per AMEND-A6; 25.3 populates them via `WithRootEnv`). envoy-go-strict cap: 64 total entries + 4096 bytes per value (hardcoded; upstream has no cap) → PARSE-REJECT + the `env_vars_cap_exceeded` counter on the stat surface (allocate-only — see below). Resolves the fixture-0036 scenario-(j) `std::env::vars()` RefCell panic (25.2 REVIEW §7 debt #4).

**Stat surface 128 → 132.** The 4 NEW counters: `vm_reload_success` / `vm_reload_runtime_failure` / `vm_reload_backoff` (Group-C upstream-parity) + `env_vars_cap_exceeded` (envoy-go-strict). See the stat-table rows above for the precise per-counter semantics. FAMILY-FINAL count.

##### envoy-go-strict departure record #8 — phase 25.3 reload-floor `base_interval = max(operator, 100ms)`

The `ReloadConfig.backoff.base_interval` (consumed when `failure_policy = FAIL_RELOAD`) is honored with an envoy-go-strict floor: `base_interval = max(operator_value, 100ms)`. Upstream applies a `JitteredLowerBoundBackOffStrategy` over the operator value (PGV `gte 1ms`) with no additional floor. envoy-go-strict imposes the 100ms floor to prevent a reload-storm (a guest that traps every request with a sub-100ms base_interval would otherwise reinstantiate the VM on a tight loop). `max_interval` is DEAD in the wasm path upstream (in the proto but unused by the wasm consumer); envoy-go mirrors the base_interval-only backoff and RETIRES the BRAINSTORM-era `max_interval` 1s-floor hypothesis as MOOT. Departure recorded per ADR-0080 + ADR-0211.

##### envoy-go-strict departure record #9 — phase 25.3 env_vars-cap 64 entries / 4096 bytes-per-value → PARSE-REJECT (`env_vars_cap_exceeded` counter is allocate-only at boot)

`VmConfig.environment_variables` (assembled `host_env_keys` + `key_values`) is capped envoy-go-strict at **64 total entries** AND **4096 bytes per value** (hardcoded, NOT operator-tunable; upstream has NO env_vars cap). Exceeding either bound → PARSE-REJECT at config-load (boot-fail-fast) with the byte-stable arm-C wording `"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"`. **Precise behavioral note: the `env_vars_cap_exceeded` counter is ALLOCATE-ONLY at boot-PARSE-REJECT.** The cap fires at config-load (`parseEnvVars`) where there is NO running per-plugin stats scope to increment a counter; the counter is ALLOCATED on the stat surface (so the surface total is 132) but is NOT incremented at config-load. This is consistent with the other 25.2 cap counters being runtime-only (the cap-exceeded events for body-buffer / shared-data / dynamic-stats fire at runtime where a per-plugin scope exists); the `env_vars_cap_exceeded` counter exists on the stat surface for future runtime use. Departure recorded per ADR-0080 + ADR-0211.

##### 6 DEFERRED conformance families (per ADR-0212 + AMEND-C5)

Phase 25.3 seeds `test/conformance/proxy-wasm/` with **10 of the 16** cpp-host (`proxy-wasm-cpp-host@da3ce05d`) UNIT-test families re-expressed as in-process Go-test sub-tests (62.5% threshold; ALL 10 PASS at phase-done with deliberate-break liveness per family). **6 families are DEFERRED** (forward-pointer roster; their absence is documented, NOT a regression):

- **shared_queue** — WasmService cross-VM queues + `proxy_on_queue_ready`; not implemented at the HTTP-filter scope.
- **signature_util** — Ed25519 signed/remote code fetch; not implemented (remote code fetch is PARSE-REJECT per the 25.1 `AsyncDataSource.Remote` departure).
- **wasm** (the cpp-host `wasm_test.cc`) — thread-local WasmHandle TLS-cache + canary; presupposes the WasmService singleton model.
- **vm_id_handle** — cross-VM scoping substrate; deferred with shared_queue/WasmService.
- **null_vm** — compiled-in NullVM engine; N/A for a Go host with no NullVM engine.
- **fuzz** — libFuzzer harnesses (not gtest); covered by envoy-go's own `FuzzWasmConfigParse` + `FuzzWasmHostcallEnvelope` Go fuzzers.

These 6 form the §9-WASM-host-family forward-pointer roster (re-evaluated when the WasmService singleton / cross-VM-queue substrate lands in a future §9 WASM-host phase). Documented in `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md`.

#### Phase 25.3 forward-pointer notes (RESOLVED from the Phase 25.2 forward-pointer notes)

**Phase 25.1 closes 0 prior-phase forward-pointers** at phase-25.1 phase-done (no prior-phase load-bearing forward-pointer was awaiting phase 25.1). Phase 25.1 self-closes a substantial in-phase RATIFIED-PENDING-IMPL list: §13-R1 (NEW `BackendKind=HTTPWasm` — landed at Task 15); §13-R3 (pairs wire format byte-faithful reimplementation — landed at Task 3); §13-R4 (WASI custom 8-stub implementation — landed at Task 4); §13-R5 D-S1 (34-fuzzer count CONFIRMED at SPEC + RATIFIED at IMPL at Task 14); §13-R7 (ADR-0196 first co-consumer — landed at Task 11 via the `EncoderFilterCallbacks.ResponseStatus()` consumer for `proxy_get_status`); §13-R8 D-P4 (STANDS WEAK-default — `ns/op = 61000` ~61µs/stream << 1ms threshold at Task 17; ADR-0205 NOT consumed); D-P1 (Task 2 — `WasiErrno::ENOTCAPABLE` = 76 matches upstream); D-P2 (Task 6 — 5 module-init/allocator UNGATED per upstream `wasm.cc:298-302`); D-P3 (Task 11 — ADR-0196 RATIFIED as first co-consumer); D-P5 (Task 9 — 18-arm byte-stable wording pinned); D-P6 (Task 16 — substring `"specifier"` chosen, DEVIATED from anticipated arm 5).

**Phase 25.2 closure summary** (all 25.1 → 25.2 BRAINSTORM scope hand-off items LANDED at 25.2 IMPL):

- **Full advanced-bridge surface delta** — LANDED in full at 25.2 IMPL (14 NEW env-namespace hostcalls + 7 NEW guest-export callbacks; body + buffer + trailers + timer + metrics + shared-data + httpCall + foreign-function + full ~70-path property all ACTIVATED). 9 envoy-go-strict counters added (BRAINSTORM hypothesized 4; AMEND-B3 expanded to 9 — counters 10-14 + the body_buffer_cap + dynamic_stats_cap + shared_data_cap counters); project stat surface 119 → 128 (BRAINSTORM hypothesized 119 → 123; AMEND-B3 + Q9 + Q6 + Q2 expanded delta to +9).
- **Per-module wazero Runtime pool design (ADR-0205 escape-valve)** — R8 RE-EVALUATED at 25.2 IMPL Task 22 `BenchmarkPerStreamModule_Instantiation` measured ~98 ns/op (well under 1ms threshold per D-P-PLAN-11 + D-25.2-P2). Under the 25.2 root-VM model the per-stream cost is bookkeeping + `proxy_on_context_create` dispatch on a shared `*RootVM.runtime` + shared compiled `*Module` (the 25.1 per-stream Runtime construction at ~61µs/stream is RETIRED). **ADR-0209 escape-valve STAYS UNCONSUMED** at 25.2 IMPL + carries forward to 25.3 IMPL escape-valve slot.
- **Foreign-function registry surface (per AMEND-A9)** — LANDED at 25.2 IMPL Task 7. `internal/wasm/foreign.go` + `internal/wasm/abi/foreign.go` + `wasm.DefaultForeignFunctionRegistry` process-global + `wasm.RegisterForeignFunction(name, fn)` API. EMPTY default registry per AMEND-A9 (upstream registers 10 defaults; envoy-go registers ZERO). Unregistered names return `WasmResult::NotFound` (=1) byte-faithful to upstream + increment `wasm.<plugin>.foreign_function_denied` counter. Concurrent dispatch via mutex-per-RootVM serialization model per D-25.2-P3 + D-P-PLAN-9.

**25.3 hand-off items — ALL RESOLVED at 25.3 IMPL phase-done (the §9 HTTP-filters family CLOSES):**

- **Per-route `typed_per_filter_config` shape (per AMEND-A3)** — RESOLVED. EXPLICIT-NO-NEW-CANONICAL per ADR-0210: wholesale `Wasm` TPFC override (no `WasmPerRoute`); ADR-0125 STAYS at 10 canonicals. Empirical finding: reference v1.37.2 has no per-route wasm support → per-route is an envoy-go capability surfaced subject-only in the differential (fixture 0038). See the 25.3 EXTENSION block above.
- **Multi-plugin-per-VM (`vm_id`-keyed VM sharing)** — RESOLVED per ADR-0211: process-global `*Registry` keyed by `Sha256(vm_id‖vm_configuration‖code)` + refcount; cross-plugin shared-data shared at raw-`vm_id` scope. See the 25.3 EXTENSION block above.
- **`VmConfig.environment_variables` activation** — RESOLVED per ADR-0211: collision-REJECT (no override) + 64-entry/4096-byte-per-value envoy-go-strict cap + WASI environ feed via `WithRootEnv`. Resolves the fixture-0036 scenario-(j) RefCell panic. Departure record #9.
- **`VmConfig.fail_open` semantics + `failure_policy = FAIL_RELOAD` + Group-C `vm_reload*` triplet** — RESOLVED per ADR-0211: `FailurePolicy` enum (UNSPECIFIED→FAIL_CLOSED; FAIL_RELOAD RuntimeError-gated; `fail_open`⊕`failure_policy` mutual-exclusivity reject); base_interval-only backoff with 100ms floor (departure record #8); `vm_reload_success`/`vm_reload_runtime_failure`/`vm_reload_backoff` triplet ACTIVATED (stat 128 → 131); `env_vars_cap_exceeded` (stat 131 → 132). See the 25.3 EXTENSION block above.
- **Conformance harness seed at `test/conformance/proxy-wasm/`** — RESOLVED per ADR-0212: in-process `go test` + vendored `.wasm` (NO Docker/Rust-in-CI); 10-of-16 cpp-host family port (62.5% threshold; all 10 GREEN with deliberate-break liveness); 6 deferred families documented (see the 25.3 EXTENSION block above + `BOOTSTRAP_PROMPT.md §7.5` + `ENVOY_TARGET.md`).
- **Per-stream Module instantiation R8 escape-valve (ADR-0209 carry-forward)** — RESOLVED: STANDS WEAK-default at 25.3 IMPL. `BenchmarkPerStreamModule_Instantiation = ~102 ns/op`; NEW `BenchmarkPerStreamPluginContextLookup = ~103 ns/op` (registry-shared VM path); NEW `BenchmarkPerRouteResolve = 0.2 ns/op` — all << 1ms. ADR-0209 + ADR-0213 reserves STAY UNCONSUMED.

**Deferred items — broader §9 WASM host family scope hand-off (CARRY FORWARD past the §9 HTTP-filters family closure):**

- **NEW `internal/filterstate/` consumer #3+ (per ADR-0207 EXPLICIT API-REVISION ALLOWANCE)** — at 25.2 consumer #1 (lua MIGRATES non-breaking) + consumer #2 (wasm); at consumer #3+ (rbac filter-state read; ext_authz inject; ext_proc pass-through; new filter families) the SPEC may revise the `*Bucket` API after empirical validation. EXPLICIT API-REVISION ALLOWANCE clause anchored in ADR-0207 §Decision body. (25.2 REVIEW debt #2 + #3 STILL deferred past 25.3 — phase-22.2 lua filterstate storage migration + the filterstate API-revision allowance.)
- **6 deferred proxy-wasm conformance families** (shared_queue, signature_util, wasm[TLS-cache], vm_id_handle, null_vm, fuzz) — re-evaluated when the WasmService singleton / cross-VM-queue substrate lands in a future §9 WASM-host phase. See the 25.3 EXTENSION block above.

**Deferred items — broader §9 WASM host family scope hand-off:**

- **EXPLICIT API-REVISION ALLOWANCE for consumer #2** (per ADR-0202 + BRAINSTORM Q3 + Q4) — `internal/wasm/` is extracted at consumer #1 (HTTP wasm filter at phase 25). Future consumers in the broader §9 WASM host family (cluster-specifier-wasm at `envoy.router.cluster_specifiers.wasm`, access-logger-wasm at `envoy.access_loggers.wasm`, network-filter-wasm at `envoy.filters.network.wasm`, WasmService singleton plugin loaders) consume the SAME primitive but MAY require API revision after empirical validation at consumer #2 (e.g., the per-stream dispatch model may not generalize cleanly from per-request HTTP filter to per-cluster cluster-specifier; the bridge-registration discipline may need a registry abstraction). The ADR-0202 §Decision body anchors the abstraction shape WITH the explicit revision allowance forward.

**No new ADR-0044 escape-valve fired at phase-25.1 IMPL (D-P4 R8 hypothesis HELD):** the SPEC-time R8 escape-valve gate (per parent §13-R8 RATIFIED-PENDING-IMPL) evaluated GREEN — `ns/op = 61000 ≤ 1_000_000` threshold; ADR-0205 NOT consumed; carries forward to 25.2 BRAINSTORM as the 25.2 IMPL escape-valve slot. 3 NEW ADR §Context + §Decision + §Consequences bodies landed at the 25.1 IMPL Task 17 atomic landing (ADR-0202 NEW `internal/wasm/` framework primitive + ADR-0203 NEW `internal/filter/http/wasm/` package shape + ADR-0204 default-deny capability sandbox; §Context anchored at parent SPEC commit `2c1455d` per ADR-0044 in-place edit discipline). ADR-0125 STAYS at 10 canonicals at this 25.1 IMPL (no per-route surface consumed at this sub-phase per AMEND-A3). The 25.1 PROGRESS Task 14 documents the 34th project-wide fuzzer `FuzzWasmConfigParse` clean at 30s/seed (3M+ execs, 67 new interesting; no panics per ADR-0018 fuzzer discipline); the Task 15 + Task 17 follow-up documents the cross-side fixture-0034 GREEN closure via the NEW `wasm.` Prometheus flattening rule at `internal/stats/name.go::flattenToProm` + the encode-side `executions` Inc removal + the scenario-(f) classifier presence-only relaxation (HCM `:scheme` / `x-forwarded-proto` / `x-request-id` injection parity gap captured as a scoped-fix TODO for a future HCM-level phase).

**No new ADR-0044 escape-valve fired at phase-25.2 IMPL (D-P-PLAN-11 + D-25.2-P2 R8 hypothesis HELD):** the IMPL-time R8 escape-valve gate (per 25.2 SPEC §15 item 41 + D-P-PLAN-11) evaluated GREEN — `BenchmarkPerStreamModule_Instantiation` reported `~98 ns/op` per stream (well under the 1ms threshold; the 25.2 root-VM model retires the 25.1 per-stream Runtime construction at ~61µs/stream — per-stream cost at 25.2 is bookkeeping + `proxy_on_context_create` dispatch on a shared `*RootVM.runtime` + shared compiled `*Module`); **ADR-0209 escape-valve STAYS UNCONSUMED** + carries forward to 25.3 IMPL escape-valve slot per the R8 signaling protocol. 4 NEW ADR §Context + §Decision + §Consequences bodies landed at the 25.2 IMPL Task 22 atomic landing per the 25.2 SPEC §10 anchor map: **ADR-0205** (root VM lifecycle evolution per Q3 — `*RootVM`/`*StreamContext` model RETIRES 25.1 per-stream `*VM`); **ADR-0206** (25.2 ABI extensions — 14 NEW env-namespace hostcalls + 7 NEW guest-export callbacks + 21 NEW capability keys gate-at-`registerCallback` per AMEND-B5 + buffer-clamp wire-contract per AMEND-B1 + metric signedness per AMEND-B2 + foreign-function dispatch concurrency mutex-per-RootVM per D-P-PLAN-9 + full ~70-path proxy_get_property roster per AMEND-B4 + EMPTY default foreign-function registry per AMEND-A9); **ADR-0207** (NEW `internal/filterstate/` framework primitive at second-consumer scope per Q7 — Bucket + FilterStateObject + StateType; consumer #1 = phase-22.2 lua MIGRATES non-breaking at Task 10; consumer #2 = phase-25.2 wasm; EXPLICIT API-REVISION ALLOWANCE for consumer #3+); **ADR-0208** (NEW `internal/filter/http/wasm/` 25.2 package extensions — 9 envoy-go-strict counters per §7.1 + AMEND-B3 incl. `http_call_response_after_close` + 4 envoy-go-strict-only config fields per Qs 2/6/9 + dynamic-stats namespace `wasmcustom.<custom_name>` per AMEND-B2 via NEW `internal/stats/dynamic/` infrastructure + fixture-0036 + fixture-0037 + 35th fuzzer + ~7-edit BEHAVIOR_CONTRACT.md bundle). **ADR-0202 §Consequences one-line in-place AMEND acknowledgment** lands at 25.2 IMPL per §10.2 (no new ADR number consumed; EXPLICIT API-REVISION ALLOWANCE for consumer #2 STAYS scoped to consumer #2; 25.2's consumer-#1-internal-scope evolutions land under NEW ADRs per phase-22.2 Q10 strict-scope precedent). ADR-0125 STAYS at 10 canonicals at 25.2 IMPL (no per-route surface consumed; AMEND-A3 RATIFIES the 5th-canonical REUSE-by-absence anticipated at 25.3 IMPL). The 25.2 PROGRESS Task 19 documents the 35th project-wide fuzzer `FuzzWasmHostcallEnvelope` clean at 30s/seed (~2.3M execs, 19 new interesting; no panics per ADR-0018 fuzzer discipline); Task 20 lands fixture-0036 mixed-mode (14 scenarios — 10 deterministic cross-side via `CompareBytes` + 4 non-deterministic subject-only via `StatsAsserter`); Task 21 lands fixture-0037 subject-only boot-reject (arm 19 `envoy_go_strict_body_buffer_cap_bytes`-zero per D-25.2-P1 closure). 22 IMPL tasks landed; 4 NEW ADR bodies + ADR-0202 AMEND; 6 phase-done gates ALL GREEN; SPEC §15 46-item acceptance ALL GREEN; 39/39 differential fixtures GREEN; 53/53 h2spec PASS at ADR-0051 v1.32.4 pin; race + vet + lint clean. **Architectural debts recorded in REVIEW.md** as 25.2-follow-up backlog (NOT load-bearing for 25.2 phase-done): phase-21 not yet migrated to NEW `internal/clock/` (extracted at Task 5 — migration follow-up); phase-22.2 lua filterstate ephemeral Bucket pattern (§14.5 non-breaking compat); fixture-0036 cross-side arms a-j skipped on Envoy v1.37.2 upstream-buffering 503 parity gap; scenario (j) env_vars deferred to 25.3 per AMEND-A6.

### Applies to
- Phase 07.1 onward (HTTP filter framework).

### Does not yet apply to
- Network filter chain (still phase 02 minimal — TCP-proxy or HCM as single entry per ADR-0033).
- HTTP filter family implementations beyond cors + envoy_go_test (incrementally landed by §9 family phases).

(REMOVED from this list — now active as of phase 07.2: listener filters, FilterChainMatch beyond SNI, and `Listener.default_filter_chain` are in scope. See `## Listener filters` below.)

---

## Listener filters

*Introduced by phase 07.2. Justified by ADR-0077 (scope), ADR-0078 (ADR-0033 partial supersession), ADR-0079 (dispatch protocol), ADR-0080 (default_filter_chain semantics; supersedes ADR-0033 clause 3), ADR-0081 (8-dimension precedence algorithm; supersedes ADR-0033 clause 2 partial), ADR-0082 (listener_filters_timeout [1s,60s] envelope), ADR-0083 (no supersession of ADR-0050; coexistence).*

Phase 07.2 lands envoy-go's listener-side filter-chain completion: the listener-filter dispatch pipeline that runs BEFORE HCM (and before any other network filter) on every accepted downstream connection, the full `FilterChainMatch` algorithm with seven match dimensions beyond SNI, and the `Listener.default_filter_chain` fallback semantics. This subsection codifies the per-connection chain-selection contract between envoy-go and reference Envoy v1.37.2.

### Asserted equivalence
- `default_filter_chain` honored as no-match fallback — verbatim scrape pinned in `### Empirical evidence (default_filter_chain fallback)` below.
- Empty-match `filter_chain` BEATS `default_filter_chain` when both coexist — verbatim scrape pinned in `### Empirical evidence (empty-match-vs-default)` below.
- `destination_port` BEATS `source_prefix_ranges` in chain-match precedence when both could match — verbatim scrape pinned in `### Empirical evidence (precedence-ordering)` below.
- `tls_inspector`-populated ALPN feeds `application_protocols` chain-match — verbatim scrape pinned in `### Empirical evidence (tls_inspector ALPN)` below (resolved at 07.2 impl time per the SPEC §11.4 carry-forward).
- 8-dimension chain-match precedence ordering: `destination_port` > `prefix_ranges` > `server_names` > `transport_protocol` > `application_protocols` > `source_type` > `source_prefix_ranges` > `source_ports`.

### Not asserted
- xDS LDS dynamic listener-filter / chain insertion / removal / reorder.
- `direct_source_prefix_ranges` chain-match dimension (proxy-protocol; silently ignored).
- `Listener.connection_balance_config`, `bind_to_port`, `reuse_port`, `transparent`, `freebind` (silently ignored).
- `listener_filters[].filter_disabled` (CEL-driven per-conn disable; silently ignored).
- `tls_inspector.enable_ja3_fingerprinting` (silently ignored).

### Dispatch protocol
- Synchronous-only (no async-resume).
- `ListenerFilter.Inspect(ctx, peeker, inputs)` returns `Continue` or `StopIteration`; on `StopIteration` the pipeline halts with whatever inputs were populated.
- Per-pipeline timeout (`Listener.listener_filters_timeout`); default 15s; honored in [1s, 60s] envelope; `continue_on_listener_filters_timeout` honored as proto-documented.
- Single-goroutine-per-connection iteration; no per-filter goroutine.
- `ListenerFilterRegistry` is boot-populated, frozen-after-boot (mirrors 07.1 `*HTTPRegistry` LBP-1 per ADR-0072).

### Chain-match algorithm
- 8 dimensions: `destination_port`, `prefix_ranges`, `server_names`, `transport_protocol`, `application_protocols`, `source_type`, `source_prefix_ranges`, `source_ports`.
- 2-pass algorithm: (1) eligibility (every non-zero dimension must match); (2) specificity scoring by priority-ordered vector.
- Tie-breakers within dimensions: longer CIDR prefix (`prefix_ranges` / `source_prefix_ranges`); SNI-specificity (exact > suffix > universal > catch-all per ADR-0033 clause 9 preserved as special case); exact value match for all other dimensions.
- Final ties (chains identical on all 8 dimensions) error at `NewManager`-build time.
- No-match → `default_filter_chain` (if set) → close conn (otherwise).

### default_filter_chain semantics
- Consulted ONLY when no `filter_chains[]` entry is eligible.
- Empty-match chain in `filter_chains[]` BEATS `default_filter_chain` when both coexist (the empty-match chain is universally eligible).
- TLS posture independent of `filter_chains[]` entries' TLS posture (mixed TLS-and-plaintext rule applies WITHIN `filter_chains[]` only).
- ADR-0033 clause 5 preserved with caveat: at most one catch-all chain in `filter_chains[]` AND at most one `default_filter_chain` (independent).

### Empirical evidence (default_filter_chain fallback)

**Probe configuration:** listener with one specific-match `filter_chain` (`source_prefix_ranges: 127.0.0.1/32`) + a `default_filter_chain`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_loopback` for the loopback-source chain; `tcp_default` for the default chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-defaultchain.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_loopback
      filter_chain_match:
        source_prefix_ranges:
        - { address_prefix: 127.0.0.1, prefix_len: 32 }
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_loopback, cluster: c_loopback } } ]
    default_filter_chain:
      name: chain_default
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_default, cluster: c_default } } ]
```

**Verbatim Envoy `/server_info` (server SHA confirmation):**

```
$ curl -s http://127.0.0.1:19901/server_info | python3 -c "import json,sys; print(json.load(sys.stdin)['version'])"
5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL
```

**Verbatim Envoy `/config_dump` (chain-shape confirmation that Envoy parsed the config without error):**

```
LISTENER l_test
  CHAIN chain_loopback fcm= {"source_prefix_ranges": [{"address_prefix": "127.0.0.1", "prefix_len": 32}]}
  DEFAULT chain_default fcm= {}
```

**Probe (a) — connection from non-loopback source (Docker NAT bridge IP):**

Driver: a TCP connection from the host's Docker bridge address (i.e., NOT 127.0.0.1 from Envoy's perspective due to user-mode NAT). The connection is closed immediately because both backend clusters point to non-listening ports — but the dispatch decision is recorded in the per-chain `downstream_cx_total` stat counter.

Verbatim stats output (`/stats?filter=tcp_(loopback|default).downstream_cx_total`):

```
tcp.tcp_default.downstream_cx_total: 1
tcp.tcp_loopback.downstream_cx_total: 0
```

**Probe (b) — connection from 127.0.0.1 (intra-container loopback):**

Driver: `docker exec envoy-pin2 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'` — inside the Envoy container so the source IP is 127.0.0.1.

Verbatim stats output after probe (b):

```
tcp.tcp_default.downstream_cx_total: 1
tcp.tcp_loopback.downstream_cx_total: 1
```

**Conclusions (pinned):**

- `default_filter_chain` is honored: a connection that doesn't match any `filter_chains[]` entry IS dispatched into the default chain (`tcp_default.downstream_cx_total` ticked from 0 → 1 on probe (a)).
- A connection that matches a specific `filter_chains[]` entry is dispatched there (NOT to the default): `tcp_loopback.downstream_cx_total` ticked from 0 → 1 on probe (b); `tcp_default` did NOT tick a second time.
- envoy-go MUST honor `default_filter_chain`: when no `filter_chains[]` entry's `filter_chain_match` matches the per-connection inputs, the algorithm falls through to `default_filter_chain` and dispatches there.

### Empirical evidence (empty-match-vs-default)

**Probe configuration:** listener with one empty-match `filter_chain` (`filter_chain_match` not set — equivalent to all-zero) + a `default_filter_chain`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_empty` for the empty-match chain; `tcp_default` for the default chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-emptyandef.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_empty
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_empty, cluster: c_a } } ]
    default_filter_chain:
      name: chain_default
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_default, cluster: c_b } } ]
```

**Boot:** Envoy starts cleanly (no parse-time error on having both an empty-match chain AND a `default_filter_chain` — the only pre-flight warning is the unrelated `connection limit` notice).

**Probe — connection from 127.0.0.1 (intra-container; would match either chain since both have no specifying dimensions):**

```
$ docker exec envoy-pin3 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'
cat: -: Connection reset by peer
```

**Verbatim stats output (`/stats?filter=tcp_(empty|default).downstream_cx_total`):**

```
tcp.tcp_default.downstream_cx_total: 0
tcp.tcp_empty.downstream_cx_total: 1
```

**Conclusions (pinned):**

- An empty-match `filter_chain` BEATS `default_filter_chain` when both coexist: `tcp_empty.downstream_cx_total` ticked from 0 → 1; `tcp_default.downstream_cx_total` STAYED AT 0.
- The `default_filter_chain` is consulted ONLY when no `filter_chains[]` entry is eligible. An empty-match chain is universally eligible (it's a chain in `filter_chains[]` with no specific dimensions to fail), so it always wins over `default_filter_chain`.
- envoy-go's chain-match algorithm MUST: (a) treat empty-match `filter_chains[]` entries as universally eligible per Pass 1 of §5.5; (b) fall through to `default_filter_chain` ONLY if zero `filter_chains[]` entries are eligible.

### Empirical evidence (precedence-ordering)

**Probe configuration:** listener with two `filter_chains[]` entries — one matching `destination_port: 10000` (the listener's own bind port; will match every connection on this listener), one matching `source_prefix_ranges: 127.0.0.1/32`. Each chain's terminal filter is a TCP-proxy with a distinct `stat_prefix` (`tcp_dstport` for the destination-port chain; `tcp_srcprefix` for the source-prefix chain). The bootstrap is at `/tmp/envoy-07.2-empirical/envoy-precedence.yaml`:

```yaml
static_resources:
  listeners:
  - name: l_test
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    filter_chains:
    - name: chain_dstport
      filter_chain_match:
        destination_port: 10000
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_dstport, cluster: c_a } } ]
    - name: chain_srcprefix
      filter_chain_match:
        source_prefix_ranges:
        - { address_prefix: 127.0.0.1, prefix_len: 32 }
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_srcprefix, cluster: c_b } } ]
```

**Probe — connection from 127.0.0.1 (intra-container; satisfies BOTH `destination_port: 10000` AND `source_prefix_ranges: 127.0.0.1/32`):**

```
$ docker exec envoy-pin4 bash -c 'exec 3<>/dev/tcp/127.0.0.1/10000 && printf "hi\n" >&3 && timeout 0.3 cat <&3'
cat: -: Connection reset by peer
```

**Verbatim stats output (`/stats?filter=tcp_(dstport|srcprefix).downstream_cx_total`):**

```
tcp.tcp_dstport.downstream_cx_total: 1
tcp.tcp_srcprefix.downstream_cx_total: 0
```

**Conclusions (pinned):**

- When both chains match a connection, `destination_port` BEATS `source_prefix_ranges`: `tcp_dstport.downstream_cx_total` ticked from 0 → 1; `tcp_srcprefix.downstream_cx_total` STAYED AT 0.
- This confirms the priority ordering documented in `filter_chain_match.proto` upstream comments and codified in §7.2: `destination_port` (priority slot 0) is more specific than `source_prefix_ranges` (priority slot 6). The chain whose specifying dimension is at a higher-priority slot wins.
- envoy-go's `chainmatch.SelectChain` MUST score chains by the priority-ordered specificity vector per §5.5 and select the highest-scoring eligible chain.

### Empirical evidence (tls_inspector ALPN)

**Status: RESOLVED at Task 16 of phase 07.2 impl session per Decision K.**

**Probe configuration:** listener with `tls_inspector` listener filter + two filter_chains discriminated by `application_protocols` (h2 vs http/1.1). Each chain has its own DownstreamTlsContext (real cert+key, ephemeral self-signed, generated at probe time only — NOT committed) advertising both ALPNs (`alpn_protocols: ["h2", "http/1.1"]`) and a per-chain TCP-proxy with distinct `stat_prefix` (`tcp_h2` for the h2 chain; `tcp_h1` for the http/1.1 chain). Bootstrap at `/tmp/envoy-07.2-impl-empirical/envoy-tls-alpn.yaml` (NOT committed; impl-time scratch directory per the SPEC §11 empirical-pin convention):

```yaml
static_resources:
  listeners:
  - name: l_tls
    address: { socket_address: { address: 0.0.0.0, port_value: 10000 } }
    listener_filters:
    - name: envoy.filters.listener.tls_inspector
      typed_config:
        "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
    filter_chains:
    - name: chain_h2
      filter_chain_match: { application_protocols: ["h2"] }
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
          common_tls_context:
            tls_certificates:
            - certificate_chain: { filename: /etc/tls/server.crt }
              private_key:       { filename: /etc/tls/server.key }
            alpn_protocols: ["h2", "http/1.1"]
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_h2, cluster: c_h2 } } ]
    - name: chain_h1
      filter_chain_match: { application_protocols: ["http/1.1"] }
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
          common_tls_context:
            tls_certificates:
            - certificate_chain: { filename: /etc/tls/server.crt }
              private_key:       { filename: /etc/tls/server.key }
            alpn_protocols: ["h2", "http/1.1"]
      filters: [ { name: envoy.filters.network.tcp_proxy, typed_config: { "@type": ".../TcpProxy", stat_prefix: tcp_h1, cluster: c_h1 } } ]
```

**Verbatim Envoy `/server_info` (server SHA confirmation — same image as §11.1–§11.3):**

```
$ curl -s http://127.0.0.1:19901/server_info | python3 -c "import json,sys; print(json.load(sys.stdin)['version'])"
5afe27fb338b16d5bb06b3a7198bcd581b4e3dee/1.37.2/Clean/RELEASE/BoringSSL
```

**Verbatim Envoy `/config_dump` (chain-shape confirmation that Envoy parsed the listener_filters[] + application_protocols match without error):**

```
LISTENER l_tls
  LFILTER envoy.filters.listener.tls_inspector
  CHAIN chain_h2 fcm= {"application_protocols": ["h2"]}
  CHAIN chain_h1 fcm= {"application_protocols": ["http/1.1"]}
```

**Probe (a) — TLS connection with `NextProtos: ["h2"]`:**

A Go probe (`probe.go` in the scratch directory) issues `tls.Dial("tcp", "127.0.0.1:19000", &tls.Config{InsecureSkipVerify: true, NextProtos: ["h2"]})`. The probe receives `cs.NegotiatedProtocol == "h2"` from `conn.ConnectionState()`, confirming the TLS handshake completed and Envoy advertised h2.

Verbatim stats output (`/stats?filter=tcp_(h2|h1).downstream_cx_total`):

```
tcp.tcp_h1.downstream_cx_total: 0
tcp.tcp_h2.downstream_cx_total: 1
```

**Probe (b) — TLS connection with `NextProtos: ["http/1.1"]`:**

Same probe with `--alpn http/1.1`. Receives `cs.NegotiatedProtocol == "http/1.1"`.

Verbatim stats output after probe (b):

```
tcp.tcp_h1.downstream_cx_total: 1
tcp.tcp_h2.downstream_cx_total: 1
```

**Verbatim `tls_inspector` per-listener-filter stats (corroborates the inspector ran and observed the ALPN extension):**

```
$ curl -s "http://127.0.0.1:19901/stats?filter=tls_inspector"
tls_inspector.alpn_found: 2
tls_inspector.alpn_not_found: 0
tls_inspector.client_hello_too_large: 0
tls_inspector.sni_found: 0
tls_inspector.sni_not_found: 2
tls_inspector.tls_found: 2
tls_inspector.tls_not_found: 0
tls_inspector.bytes_processed: P0(nan,1400) P25(nan,1425) P50(nan,1450) P75(nan,1475) P90(nan,1490) P95(nan,1495) P99(nan,1499) P99.5(nan,1499.5) P99.9(nan,1499.9) P100(nan,1500)
```

**Conclusions (pinned):**

- An HTTPS connection offering `NextProtos: ["h2"]` is dispatched to `chain_h2` (the chain whose `application_protocols: [h2]` matches): `tcp_h2.downstream_cx_total` ticked from 0 → 1 on probe (a); `tcp_h1.downstream_cx_total` STAYED AT 0.
- An HTTPS connection offering `NextProtos: ["http/1.1"]` is dispatched to `chain_h1` (the chain whose `application_protocols: [http/1.1]` matches): `tcp_h1.downstream_cx_total` ticked from 0 → 1 on probe (b); `tcp_h2.downstream_cx_total` STAYED AT 1 (did NOT tick further).
- The `tls_inspector` per-filter counters confirm the listener filter ran on both connections (`tls_found: 2`, `alpn_found: 2`, `sni_not_found: 2` — neither probe sent SNI). This is the empirical demonstration that `tls_inspector` populates `inputs.ApplicationProtocols` from the ClientHello's ALPN extension; without `tls_inspector` in `listener_filters[]`, `application_protocols` chain-match cannot fire (the chain-match algorithm has no source of ALPN data otherwise).
- envoy-go MUST run the `tls_inspector` listener filter BEFORE chain-match selection so that `inputs.ApplicationProtocols` is populated when chains discriminate on `application_protocols`. The §13.1 BEHAVIOR_CONTRACT integration enforces this at the phase-done commit (Task 17). ADR-0083 confirms this is orthogonal to ADR-0050's HCM-internal AUTO-codec dispatch — the chain-match `application_protocols` is what selects the chain, not the codec; the chain's terminal filter (HCM with forced `codec_type`, or here a TCP-proxy as the minimal probe) consumes the chain selection result.

### Applies to
- Phase 07.2 onward (listener filters + 8-dimension `FilterChainMatch` + `default_filter_chain`).

### Does not yet apply to
- Listener filters beyond `tls_inspector` (`original_dst`, `proxy_protocol`, `http_inspector` deferred — SPEC §2.1).
- xDS LDS dynamic listener configuration (xDS family).
- `direct_source_prefix_ranges` chain-match dimension (bundled with proxy-protocol filter phase).
- HTTP/3 + QUIC listener filters (HTTP/3 family).

---

## Network filters

*Introduced by phase 26.1 — the FIRST §9 Network-filters-family row, opening the first new §9 feature family since the §9 HTTP-filters family closed at phase 25. Justified by ADR-0213 (L4 read-filter chain framework) + ADR-0214 (network-filter registry + boot-wiring + 26.1 dual-dispatch).*

Phase 26.1 lands the NEW `internal/filter/network/` read-filter chain framework — the L4 analogue of phase 07.1's HTTP filter framework, and the first new filter-category framework primitive since phase 07.1 — plus its first two consumers (`echo` + `direct_response`). This subsection codifies the read-filter iteration contract and the two filters' wire behavior against reference Envoy v1.37.2.

At master tip the only network filters (`tcp_proxy` + HCM) were terminal-only, selected one-per-chain via a hardcoded `map` in `internal/listener/manager.go` with a private `Handle(ctx, conn)` interface — no iteration, no callbacks, no read-filter protocol. Phase 26.1 introduces the missing chain framework alongside the untouched terminal path (the **dual-dispatch**, below); `tcp_proxy`/HCM migrate onto the read-filter interface and the hardcoded registry retires at 26.2.

### Network filter chain framework

The read-filter chain runner (`internal/filter/network/`) iterates read filters over an accepted downstream connection, mirroring Envoy v1.37.2 `source/common/network/filter_manager_impl.cc` at L4.

- **Iteration-status protocol.** The `ReadFilter` interface is `OnNewConnection() Status` + `OnData(buf []byte, endStream bool) Status` (+ `SetReadFilterCallbacks(cb)` + `OnDestroy()`), returning a **TWO-value** `Status` enum (`Continue` / `StopIteration`). There is NO `StopIterationAndBuffer` / `StopAllIteration` variant (unlike the HTTP chain's `FilterDataStatus`) because L4 buffering is CONNECTION-level, not filter-level (empirical scrape of `envoy/network/filter.h` — the network `FilterStatus` enum has exactly two values).
- **`OnNewConnection`** is called eagerly per filter at connection accept (before any data), in registration order, stopping on `StopIteration` (mirrors upstream `initializeReadFilters()`).
- **`OnData`** is called with the connection read buffer when `length > 0 || endStream`.
- **Connection-level buffering + re-iteration semantics.** On `StopIteration` the chain runner stops at the current filter and leaves the undrained bytes in the connection read buffer. There are TWO distinct halt behaviors: an `OnNewConnection`-`StopIteration` is **sticky across reads** (the only cross-read-persistent halt; cleared only by `ContinueReading()`); an `OnData`-`StopIteration` is **NOT sticky** — it stops the current pass only, and the NEXT socket read re-delivers the accumulated buffer from the stopping filter (upstream `onRead` re-iteration). This distinction is load-bearing: `echo` returns `StopIteration` from every `OnData` (without `ContinueReading`) yet must echo EVERY subsequent read, not just the first.
- **`ContinueReading()`** resumes at the NEXT filter with the currently-available buffered bytes (upstream `onContinueReading(...)` resuming at `std::next(filter->entry())`).
- **`ReadFilterCallbacks` surface:** a `Connection()` accessor (`Write([]byte, endStream)`, `Close(CloseType)`, `LocalAddr()/RemoteAddr() net.Addr`, `RequestedServerName() string`, `DownstreamPrincipals() []string` — the full L4 accessor surface shaped at 26.1 so 26.3 `rbac_network` needs no callbacks revision), `ContinueReading()`, and `DynamicMetadata() *dynamicmetadata.Bucket`.
- **`CloseType`** enum has `FlushWrite` / `NoFlush` at 26.1 (no other variants needed by the first consumers).
- **Per-connection runtime context** — the genuinely-new L4 primitive (analogue of the HTTP chain's per-stream state). It owns the reused `*dynamicmetadata.Bucket` (`internal/dynamicmetadata/`; constructed via `NewBucket()` at connection entry, `Reset()`+nil at `OnDestroy`) and is threaded into each filter's callbacks. NO filter writes to the Bucket at 26.1 (the first real `Set` lands at 26.3 `rbac_network`); it round-trips a unit-test `Set`/`Get` for accessor readiness.
- **Single-goroutine-per-connection** — the chain runner dispatches synchronously on the connection goroutine (`internal/listener/manager.go serveNetworkChain`), consistent with the existing model. No per-filter goroutine; cross-goroutine async read-filter resume is deferred.
- **Drainable `Buffer`** — the read buffer models upstream's `Buffer::Instance` drain semantics (`Append`/`Bytes`/`Len`/`Drain`; over-drain clamped) that `echo`'s write-back-then-drain relies on.
- **Freeze-after-boot `*network.Registry`** — boot-populated, frozen before serving, lock-free post-Freeze lookup (see ADR-0214 below).
- **Unified single-dispatch path (26.2 — the dual-dispatch retired).** At 26.2 ALL four built-in network filters (`echo`/`direct_response` + the migrated `tcp_proxy`/HCM) resolve through the frozen `*network.Registry`; `serveConnection` step-7 is ONE `serveNetworkChain` call (the 26.1 dual-branch is gone). The 26.1 build-time pre-check (`filters[0]` type_url resolves in `netReg` → new path else old terminal path) is RETIRED: every chain is a registry chain. R3 back-compat is proven by the existing `0000-tcp-echo` + `0001-tcp-proxy-rr` + `0002-tls-tcp` + HCM (`0003-http11-routing`, `0004-h2-routing`, …) differential fixtures + conformance + h2spec staying byte-exact green. See the 26.2 framework-update block below.

**envoy-go-strict departure records (joint divergence-window with the prior §9 family rows):**

- **Write-filter ABSENT (ADR-0213).** Only read filters are supported (26.1 onward); the `WriteFilter` / `onWrite` surface is DEFERRED with an EXPLICIT API-REVISION ALLOWANCE clause (every near-term Network-family filter — `echo`, `direct_response`, `tcp_proxy`, `rbac_network`, `sni_cluster` — is a read filter; write filters appear only in deferred protocol proxies). Building unexercised write-filter plumbing would violate the every-surface-exercised discipline.
- **Read+terminal scope.** The framework now expresses BOTH a read-filter (`OnData`) model and a terminal-filter (connection-takeover) model (26.2); `rbac_network` connection-metadata writes are still 26.3.

### Network filter chain framework — 26.2 framework update (terminal-filter seam + dispatch unification)

*Updated by phase 26.2 (`network-filter-registry-migration`). Justified by ADR-0215 (the `tcp_proxy`/HCM migration onto the framework + the `manager.go` hardcoded-registry retirement + the dispatch unification — at byte-exact parity). NO operator-visible behavior change to `tcp_proxy`/HCM: each filter's `Handle(ctx, conn)` connection-handling loop is UNTOUCHED.*

Phase 26.2 migrates the two existing terminal network filters — `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) — onto the `internal/filter/network/` framework + the freeze-after-boot `*network.Registry`, retiring the hardcoded terminal-filter machinery in `internal/listener/manager.go` and unifying the per-connection dispatch into a single registry-driven path. The structural-debt-paydown sub-phase that proves the 26.1 framework on the load-bearing path.

- **`TerminalFilter` connection-takeover seam.** Alongside the `ReadFilter` (`OnData`) model, the framework gains `network.TerminalFilter` — an interface whose single method `Handle(ctx context.Context, downstream net.Conn)` takes over the raw downstream connection and runs to connection close. It is byte-identical to the retired `manager.go` `filterHandler`, so `*tcpproxy.Filter` + `*hcm.Filter` satisfy it with ZERO method changes (their `Handle` loops — tcp_proxy's L4 bidirectional `io.Copy` pump; HCM's ALPN-dispatched H1 driver / H2 `ServerConn` codec — are untouched → byte-exact parity is intrinsic). This is the FIRST time the L4 framework expresses connection ownership (vs `echo`/`direct_response` writing *through* `ReadFilterCallbacks.Connection().Write` while the framework owns `conn.Read`).
- **Sealed `NetworkFilter` marker + generalized factory.** A `NetworkFilter` marker interface (= `ReadFilter` | `TerminalFilter`) is SEALED via the exported zero-size `network.Marker` embeddable (Go's cross-package interface-sealing idiom — out-of-package filters embed `Marker` to satisfy the unexported `isNetworkFilter()`). The registry's per-connection factory generalizes to `FilterInstanceFactory func() NetworkFilter`.
- **`[read-filter*, terminal-filter?]` chain shape.** The chain runner classifies a built `[]NetworkFilter` into a read-filter prefix + an OPTIONAL trailing terminal filter. A pure-read chain runs the read loop to EOF; a pure-terminal chain hands the conn straight to `Handle`; a mixed chain runs the read prefix and, when a read filter `Continue`s past the prefix, hands the connection to the terminal via the buffered-prefix `prefixConn` handover (the bytes a preceding read filter inspected but did not consume are replayed to the terminal before the live socket bytes).
- **Unified dispatch (the dual-dispatch retired).** `serveConnection` step-7 is now ONE `serveNetworkChain` branch resolving every chain through `netReg` (the 26.1 step-7 dual-branch + the build-time pre-check are gone). The hardcoded terminal machinery in `manager.go` is RETIRED: the `filterRegistry` map, the `filterConstructor` type, the private `filterHandler` interface, `buildTerminalFilter`, and the per-chain `listenerCtx` build context are all DELETED; `netReg` is intrinsic (the 26.1 nil-tolerance "nil netReg → old terminal path" is removed — there is no old path). All four built-ins resolve through `*network.Registry`.
- **Registration seam.** A NEW `internal/filter/network/builtins` package exposes `RegisterBuiltins(reg, Deps)` registering all four built-ins (`echo`/`direct_response`/`tcp_proxy`/HCM); placement is import-cycle-forced (it imports echo/directresponse/tcpproxy/hcm/network, so it cannot live in `network`/`listener`/`cmd`). The heavy boot singletons (`*cluster.Manager`, `*drain.Manager`, `*stats.Registry`, `[]accesslog.Sink`, `*filter_http.HTTPRegistry`, `*httpclient.Client`) are closure-captured in the `tcpproxy.NewNetworkFactory` / `hcm.NewNetworkFactory` registration adapters so `internal/filter/network` stays import-light (no `cluster`/`stats`/`hcm` imports). The per-chain primitives (`HasTLS`/`AllowH2C`/`ListenerPrincipal`/`NodeServiceCluster`) are added to `network.FactoryCtx`.
- **The `network-filter-mixed-chain-unsupported` reject is LIFTED.** With `tcp_proxy`/HCM now network filters, mixed `[read-filter*, terminal-filter]` chains are EXPRESSIBLE (the 26.1-transitional reject — that REJECTED a chain whose `filters[0]` resolved in `netReg` but whose subsequent filters did not all resolve — no longer exists). At 26.2 NO shippable read filter `Continue`s to a terminal (`echo` halts via `StopIteration` + never `ContinueReading`s; `direct_response` writes + closes), so the mixed path is exercised by a UNIT test (a synthetic always-`Continue` read filter → a recording terminal asserting the `prefixConn` buffered-prefix handover). The FIRST production consumer of a mixed read→terminal chain is `rbac_network` (26.3: allow → `Continue` → `tcp_proxy`/HCM); the parent §8.2-anticipated "echo preceding tcp_proxy" differential fixture is DEFERRED to 26.3 (echo would never advance to the terminal).
- **NEW chain-shape boot-rejects.** The unified builder validates the `[read*, terminal?]` shape with two NEW envoy-go-strict boot-rejects: `network-filter-terminal-not-last` (a terminal filter — `tcp_proxy`/HCM — appears before the end of the chain) and `network-filter-multiple-terminals` (more than one terminal filter in a chain). These are UNIT-TEST-only boot-rejects (no cross-side differential fixture). The byte-stable unknown-type-url reject wording is PRESERVED verbatim (`"%s: unknown filter type_url %q"` — ADR-0080).
- **Terminal-filter BUILD errors now carry a `filters[N]:` index segment.** Because `tcp_proxy`/HCM now build through the unified chain path, a `tcp_proxy`/HCM build error is now prefixed with its position in the chain (e.g. `... filter_chains[0]: filters[0]: hcm: ...`) where the pre-26.2 terminal path emitted `... filter_chains[0]: hcm: ...`. This is an INTENDED user-observable wording change (the `filters[N]:` segment) — a consequence of routing `tcp_proxy`/HCM through the unified chain builder. It is distinct from the byte-stable *unknown filter type_url* reject (whose wording is unchanged). The change pre-existed in the net-chain build path since 26.1; 26.2 extends it to the `tcp_proxy`/HCM build errors.
- **Read-filter iteration contract UNCHANGED.** The `OnNewConnection`/`OnData` → `Continue`/`StopIteration` two-value protocol, the sticky-vs-non-sticky halt semantics, the connection-level re-iteration, the `ReadFilterCallbacks` surface, and the per-connection runtime context (owning the reused `*dynamicmetadata.Bucket`) are all unchanged from 26.1.

### envoy.filters.network.echo

`echo` (`@type` `type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo` — see the type-URL note below) reflects downstream bytes back over the same connection. Config is the EMPTY `Echo` message (zero user fields; vacuous PGV); an empty or absent `typed_config` body is accepted (no field-level PARSE-REJECT; no echo PARSE-REJECT arms).

- **`OnNewConnection`** — not overridden / returns `Continue` (mirrors `echo.cc`, which has no `onNewConnection` override).
- **`OnData(buf, endStream)`** — `Connection().Write(buf.Bytes(), endStream)` then `buf.Drain(buf.Len())` then `return StopIteration` (mirrors `echo.cc`: `connection().write(data, end_stream)` + `ASSERT(0 == data.length())` + `FilterStatus::StopIteration`). Echoes downstream bytes verbatim; the read loop continues (re-delivering each subsequent read per the non-sticky `OnData`-`StopIteration` semantics above) until the downstream closes (EOF).
- **`SetReadFilterCallbacks` / `OnDestroy`** — store cb; `OnDestroy` is a no-op (no per-connection resources).
- **Stats:** 0 built-in counters (upstream parity).
- **Differential:** cross-side fixture `0040-network-echo` — a multi-write payload echoed byte-exact vs reference Envoy v1.37.2 (the empirical confirmation that the corrected `@type` boots on real upstream).

### envoy.extensions.filters.network.direct_response

`direct_response` (`@type` `type.googleapis.com/envoy.extensions.filters.network.direct_response.v3.Config`) writes a static configured response and closes the connection. NOTE the proto message is named **`Config`** (not `DirectResponse`).

- **Config — single field `response`** (`config.core.v3.DataSource`, tag 1). `New(tc, ctx)` resolves the `response` DataSource at config-load (boot) into the static response bytes, via the 4-arm `DataSource.specifier` oneof: `inline_string` → byte-cast; `inline_bytes` → verbatim; `filename` → `os.ReadFile` relative to the bootstrap base dir (`FactoryCtx.BaseDir`); `environment_variable` → `os.LookupEnv`. A bad `filename` / unset env-var rejects at boot.
- **`OnNewConnection`** — the logic lives HERE (NOT `OnData`): `Connection().Write(responseBytes, true)` (endStream=true) + set the internal response-code-details string `DirectResponse` (below) + `Connection().Close(FlushWrite)` + `return StopIteration`. Mirrors `direct_response/filter.cc`: `connection.write(data, true); connection.close(FlushWrite); return StopIteration`. NO configurable delay (v1.37.2).
- **`OnData`** — not exercised in the normal path (the connection closes in `OnNewConnection` before any data iteration); returns `StopIteration` defensively.
- **Internal `DirectResponse` response-code-details string — NO operator-visible surface (envoy-go-strict, set-but-unread).** Unlike the HTTP path there is no `streamInfo`-equivalent sink on an L4 connection at master tip, so the string has no existing operator-visible surface. envoy-go sets it (on the per-connection runtime context's RCD sink) for upstream-parity + forward-consumer readiness; it is set-but-unread at 26.1, with no fixture assertion (joint divergence-window with the prior §9 rows).
- **Boot-reject parity (`response.specifier` required).** A `direct_response` config whose `response.specifier` oneof is unset rejects at boot in BOTH binaries. envoy-go's byte-stable wording is `direct_response: response.specifier is required`; reference Envoy v1.37.2 emits `ConfigValidationError.Response: ... field: "specifier", reason: is required` — the shared case-sensitive substring is **`specifier`** (cross-side fixture `0042-network-direct-response-boot-reject`). The `response`-ABSENT arm does NOT reject upstream (boots), so `response: {}` (specifier unset) is the only symmetric boot-reject arm.
- **Stats:** 0 built-in counters (upstream parity).

### envoy.extensions.filters.network.rbac

*Introduced by phase 26.3 (`network-filter-rbac`) — the THIRD §9 Network-filters-family sub-phase + the FAMILY's first L4 policy filter. Justified by ADR-0216 (the shared `internal/rbac/` engine extraction + the input-capability `Profile`), ADR-0217 (the FIRST production WRITE through the connection-scoped `*dynamicmetadata.Bucket`), and ADR-0218 (the `rbac_network` filter itself).*

`rbac_network` (`@type` `type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC` — type URL DERIVED via `proto.MessageName`, NOT hand-typed; the network-filter type URLs carry the `extensions.` segment) enforces L4 connection-level RBAC: an ALLOW/DENY decision over the four-tuple + SNI + mTLS-authenticated identity, decided in `OnData` on the first byte. It is **consumer #2** of the shared `internal/rbac/` engine (HTTP rbac is consumer #1) and the FIRST production consumer of both the connection-scoped dynamic-metadata bucket (ADR-0217) and a mixed read→terminal chain (allow → `Continue` → `tcp_proxy`/HCM via the `prefixConn` handover shaped at 26.2 — R-M-LIVE).

- **L4 input surface (ProfileL4).** The shared engine compiles under `ProfileL4`, which PARSE-REJECTS the HTTP-only permission/principal arms at compile. SUPPORTED at L4: **permissions** — `any`, `destination_ip`, `destination_port`, `destination_port_range`, `requested_server_name`, plus the `and_rules` / `or_rules` / `not_rule` combinators; **principals** — `any`, `authenticated` (mTLS principal-name candidates: URI SAN → DNS SAN → Subject CN, the priority-ordered list shaped at phase 16 / ADR-0144 and exposed at L4 via `Connection().DownstreamPrincipals()`), `direct_remote_ip`, `remote_ip`, plus the `and_ids` / `or_ids` / `not_id` combinators. REJECTED at compile (`ProfileL4`): `permission.header`, `permission.url_path`, `principal.header`, `principal.url_path` (HTTP-only — byte-stable wording `rbac: <arm> is HTTP-only (unsupported for L4 network RBAC)`, pinned by `TestProfileL4RejectWording_ByteStable`). `source_ip` rejects unconditionally (it is a PARSE-REJECT arm in `buildOnePrincipal` regardless of profile).
- **Decision-in-`OnData` + `OnNewConnection` Continue no-op (sticky-halt constraint).** `OnNewConnection` MUST return `Continue` — a `StopIteration` there sets the chain's STICKY `connHalted` flag and blocks ALL subsequent `OnData` calls (memory `reference_network_read_filter_onnewconnection_halts`), so the decision would never run. The decision runs in `OnData` (AMEND-A8): `ONE_TIME_ON_FIRST_BYTE` (the default `enforcement_type`) decides once — subsequent `OnData` calls pass through via a `decided` guard; `CONTINUOUS` re-decides every `OnData`. A wholly-inactive filter (neither enforced engine set; `rbac.pb.go:33` "If absent, no RBAC enforcement occurs.") is passthrough `Continue` with NO counters (mirrors the HTTP consumer's BOTH-nil gate; `decided` is NOT set).
- **Enforced-deny = NoFlush close + `rbac_deny_close` termination-detail.** An enforced DENY sets response-code-details `rbac_deny_close` (on the per-connection RCD sink, via the optional `SetResponseCodeDetails(string)` interface — byte-faithful to upstream's `setConnectionTerminationDetails("rbac_deny_close")`), calls `Connection().Close(NoFlush)`, and returns `StopIteration` (AMEND-A7). The `NoFlush` close is the F3 framework touchpoint (see the NoFlush-now-distinguished note below). An enforced ALLOW returns `Continue` (advancing to the terminal / `prefixConn` handover).
- **Shadow = dynamic-metadata shadow-pair + `shadow_*` stats.** When a shadow engine is configured (`shadow_rules` wins over `shadow_matcher`), the shadow walk (which NEVER affects the enforced disposition) ticks the `shadow_allowed` / `shadow_denied` counter AND writes the shadow pair into the per-connection dynamic-metadata bucket under namespace **`envoy.filters.network.rbac`**: key `shadow_engine_result` (value `"allowed"` / `"denied"`) and key `shadow_effective_policy_id` (the matched policy id — written ONLY when non-empty, upstream parity). This is the FIRST production WRITE through the connection-scoped `*dynamicmetadata.Bucket` (ADR-0217; REUSE of the connection-scoped bucket — no `internal/dynamicmetadata/` code change, R5). The bucket is nil-receiver tolerant (ADR-0085).
- **The rules / matcher dual-path.** Both the enforced and shadow engines select rules-OR-matcher independently (`rules` wins over `matcher`; `shadow_rules` over `shadow_matcher`; both unset → wholly inactive), mirroring the HTTP consumer's dispatch + §1.1 amendment 2. The matcher-path carries no rbac-permission/principal leaf, so its HTTP-only-arm reject is vacuously satisfied (D-26.3-1).
- **CEL / audit silent-ignore (inherited from the engine).** The CEL `condition` / `checked_condition` / `cel_config` and the audit-logging surface are SILENTLY IGNORED by the phase-16 engine (ADR-0040); `rbac_network` MIRRORS the silent-ignore (D-P4 — the parent §6.3 `condition-cel-unsupported` reject arm was RETRACTED at the 26.3 SPEC). F1.
- **The four-counter roster (NO per-policy F2).** `<stat_prefix>.rbac.{allowed,denied,shadow_allowed,shadow_denied}` — see the stat-table extension block above (132 → 136). The network RBAC proto has no `track_per_rule_stats`, so the engine's per-policy machinery stays dormant for consumer #2 (D-26.3-7 + D-P5).

**envoy-go-strict departure records (joint divergence-window with the prior §9 family rows):**

- **HTTP-only-matcher PARSE-REJECT (AMEND-A4).** The four HTTP-only permission/principal arms (`permission.header`, `permission.url_path`, `principal.header`, `principal.url_path`) PARSE-REJECT at compile under `ProfileL4` rather than evaluating-to-no-match. Upstream's network RBAC would compile them (and they would never match at L4); envoy-go rejects at boot so a misconfigured L4 policy fails fast. Byte-stable wording pinned by `TestProfileL4RejectWording_ByteStable`.
- **`delay_deny` PARSE-REJECT (AMEND-A9).** The deferred-deny timer surface (`delay_deny`) is unsupported at envoy-go MVP — set → boot-reject (`rbac_network: delay_deny is unsupported`, pinned by `TestParseRejectConstants_ByteStable`).
- **xDS dynamic-policy PARSE-REJECT.** Dynamic (xDS-delivered) RBAC policy is unsupported; only the static `typed_config`-embedded rules/matcher compile. (Consistent with the project-wide static-config MVP boundary.)
- **`stat_prefix`-invalid + `shadow_rules_stat_prefix`-invalid PARSE-REJECT.** A `stat_prefix` (or `shadow_rules_stat_prefix`) containing characters invalid for a metric name is rejected at boot (`rbac_network: stat_prefix contains characters invalid for a metric name` / `rbac_network: shadow_rules_stat_prefix contains characters invalid for a metric name`) — validated at the input boundary before `NewCounterIfAbsent` (which panics on invalid names). This is CONSISTENT with how hcm/lua document their `stat_prefix` validation as envoy-go-strict (ADR-0059). Both wordings (plus the PGV-required `stat_prefix is required`) pinned by `TestParseRejectConstants_ByteStable`. (The two stat_prefix-invalid consts were added at Task 13 by the fuzzer-found `stat_prefix`-validation fix.)
- **Connection-metadata emitted-but-unread (AMEND-A5/A6).** The shadow-pair dynamic-metadata writes (`shadow_engine_result` / `shadow_effective_policy_id`) are EMITTED at the connection-scoped bucket but have NO reader yet at 26.3 (no L4 access-logger / downstream filter consumes connection dynamic-metadata at master tip) — emitted-but-unread for upstream-parity + forward-consumer readiness, joint divergence-window with the `direct_response` `DirectResponse` rcd set-but-unread record. The `rbac_deny_close` rcd is similarly written to the per-connection RCD sink with no operator-visible L4 surface at master tip.

**Structural / framework notes:**

- **`internal/rbac/` engine-extraction (ADR-0216).** The phase-16 RBAC evaluator surface (the abstract `EvalContext` + the ~23 permission/principal evaluators + the rules/matcher compilers + the matcher bridge + the per-policy machinery, ~790 LoC) was MOVED to a shared `internal/rbac/` package and re-exported, gaining an input-capability `Profile` (`ProfileHTTP` / `ProfileL4`). **HTTP rbac = consumer #1, re-verified byte-exact** (it passes `ProfileHTTP` → byte-identical compile + evaluation to pre-extraction; the phase-16 HTTP-rbac differential fixtures stay byte-exact green — R4, the LIVE-first-consumer re-verification discipline). **NO HTTP-rbac behavior change.** `rbac_network` = consumer #2 (passes `ProfileL4`). The engine stays the single owner of arm-classification (R-E).
- **NoFlush-now-distinguished (F3).** Phase 26.3 is the first PRODUCTION consumer to call `Connection().Close(NoFlush)` (the enforced-deny path) — the F3 framework touchpoint distinguishing `NoFlush` from `FlushWrite` (which `direct_response` uses). The `CloseType` enum's two variants now both have a production consumer.

**Differential coverage + known boundary:**

- **Cross-side fixtures.** Phase 26.3 adds 2 differential fixture dirs (44 → 46): a cross-side allow/deny/shadow `StatsAsserter` fixture (R-M-LIVE — the FIRST production mixed read→terminal chain proven cross-side: `rbac_network` allow → `Continue` → `tcp_proxy` terminal) + a boot-reject fixture. The phase-16 HTTP-rbac differential fixtures are re-run byte-exact (R4) to prove the engine extraction is behavior-neutral for consumer #1.
- **D-26.3-4 SNI/mTLS differential gap (KNOWN coverage boundary).** The L4 SNI accessor (`requested_server_name`) + the mTLS-authenticated principal accessor (`Connection().DownstreamPrincipals()`) are UNIT-TESTED (Task 9) but NOT cross-side differential-proven (no reference-Envoy fixture exercises a `requested_server_name` / `authenticated` L4 RBAC policy end-to-end). The `destination_port` / `destination_ip` / `direct_remote_ip` / `remote_ip` / `any` arms ARE differential-proven; the SNI/mTLS arms rest on unit coverage. Recorded as a known coverage boundary (PROGRESS.md + ADR-0218 §Consequences).

### envoy.filters.network.sni_cluster

*Introduced by phase 27 (`network-filter-sni-cluster`) — the SECOND §9 Network-filters-family row (a flat top-level row per ADR-0106) + the FAMILY's first routing-steering read-filter. Justified by ADR-0219 (the connection-scoped upstream-cluster-override seam + the `tcp_proxy` per-connection resolution) and ADR-0220 (the `sni_cluster` filter itself).*

`sni_cluster` (`@type` `type.googleapis.com/envoy.extensions.filters.network.sni_cluster.v3.SniCluster` — type URL DERIVED via `proto.MessageName`, NOT hand-typed; the network-filter type URLs carry the `extensions.` segment) is a config-less L4 read-filter that publishes the TLS SNI verbatim as a per-connection upstream-cluster-override the terminal `tcp_proxy` consumes to select its upstream cluster. The `SniCluster` proto is an EMPTY message (zero user fields; no PGV); an empty or absent `typed_config` body is accepted (the `echo` template — a malformed `Any` surfaces `sni_cluster: invalid typed_config: %w`). It is the 6th built-in network filter and the SECOND production mixed read→terminal chain (after 26.3 `rbac_network`).

- **`OnNewConnection`** — reads `Connection().RequestedServerName()`; when the SNI is non-empty, publishes it VERBATIM (no lowercasing/trimming/truncation) as the per-connection upstream-cluster-override via `SetUpstreamCluster(sni)`; always returns `Continue` (mirrors `sni_cluster.cc::onNewConnection()`, which sets the `PerConnectionCluster` filter-state key `envoy.tcp_proxy.cluster` verbatim and returns `Continue`). It MUST return `Continue`, not `StopIteration` — an `OnNewConnection` `StopIteration` sets the chain's sticky `connHalted` flag that blocks ALL `OnData` (memory `reference_network_read_filter_onnewconnection_halts`); Envoy's filter also returns `Continue`, so parity and the framework constraint coincide.
- **`OnData(buf, endStream)`** — pass-through `Continue` (NOT a halt, NOT a drain): `sni_cluster` does not inspect payload, so the inspected bytes flow to the terminal (the `prefixConn` handover replays any pre-terminal bytes). This makes `[sni_cluster, tcp_proxy]` a mixed read→terminal chain (`sni_cluster` `Continue`s from `OnNewConnection`, `TerminalReady()` fires, `HandleTerminal` threads the override). The first config-less read filter that `Continue`s to a terminal (`rbac_network` decides in `OnData`; `sni_cluster` publishes in `OnNewConnection`).
- **`SetReadFilterCallbacks` / `OnDestroy`** — store cb; `OnDestroy` is a no-op (no per-connection resources; the per-connection runtime is discarded after dispatch).
- **Empty / absent SNI → no override.** When `RequestedServerName()` is empty, `sni_cluster` sets NO override → `tcp_proxy` uses its configured cluster (fallback; F-RESOLVE). Both the `OnNewConnection` `sni != ""` guard and the `tcp_proxy` `Handle` `override != ""` guard are redundant-by-design (defense-in-depth — an empty override can never route to a literal `""` cluster).
- **Stats:** 0 built-in counters (upstream parity — `sni_cluster` only publishes the override).
- **No per-route surface.** The proto carries no per-route message; network filters have no `typed_per_filter_config` surface (D27-7).
- **Differential:** cross-side TLS fixture `0045-sni-cluster` — 3 arms (route: SNI naming an existing cluster → routed to that backend; fallback: empty/absent SNI → `tcp_proxy`'s configured cluster; unknown-cluster-close: SNI naming a non-existent cluster → downstream close, zero bytes) byte-exact vs reference Envoy v1.37.2. The route/fallback arms are made non-vacuous by a `DistributionAsserter` (per-backend accept counts `[1, 1]` — a broken override that routes everything to the fallback gives `[0, 2]` and FIRES). The unknown-cluster-close arm normalizes the TLS-handshake lifecycle difference (reference closes pre-handshake via tls_inspector; envoy-go extracts SNI from the completed `*tls.Conn` handshake then closes post-`tcp_proxy.Handle`) — both produce zero application bytes (`closeOK`, within D27-S3).

### envoy.filters.network.zookeeper_proxy

*Introduced by phase 28.1 (`network-filter-write-seam-and-zookeeper-requests` [28.1a] + `network-filter-read-seam-and-zookeeper-requests-proof` [28.1b]) — the THIRD §9 Network-filters-family row (a flat top-level row per ADR-0106) + the family's first stats-PRIMARY filter + the first both-direction (`ReadFilter` + `WriteFilter`) consumer. Justified by ADR-0221 (the terminal-handoff conn-wrap seam, both directions) + ADR-0222 (the `zookeeper_proxy` request side). Cross-side-PROVEN by the now-green `0046-zookeeper-requests` fixture (28.1b).*

`zookeeper_proxy` (`@type` `type.googleapis.com/envoy.extensions.filters.network.zookeeper_proxy.v3.ZooKeeperProxy` — type URL DERIVED via `proto.MessageName`, NOT hand-typed; the `extensions.` segment is intrinsic) is a passive observability sniffer that decodes the ZooKeeper client protocol (jute framing) flowing through a `[zookeeper_proxy, tcp_proxy]` chain and emits per-opcode counters. It NEVER mutates bytes, never halts iteration, never closes — its entire operator-visible surface IS its stats (the body differential is intrinsically vacuous; the cross-side `StatsAsserter` is the load-bearing proof). It is the 7th built-in network filter and implements BOTH `ReadFilter` and `WriteFilter` as ONE instance (the first consumer of the ADR-0221 seam).

- **Request-side semantics (28.1).** `OnNewConnection` is a no-op `Continue` (an OnNewConnection `StopIteration` would set the sticky `connHalted` flag — `reference_network_read_filter_onnewconnection_halts`). `OnData` feeds the shallow decoder + ALWAYS returns `Continue` (unconditional passthrough; R3 — never drains/mutates/halts/closes). `OnWrite` is a PURE no-op `Continue` at 28.1 (the response decoder lands at 28.2 / ADR-0223); the no-op `OnWrite` exists so the filter satisfies `WriteFilter` (→ the shared wrap predicate → the read seam) and so the `0046` traffic exercises the `writeChainConn → OnWrite` seam end-to-end. The shallow request decoder does BE length-prefix framing + xid sniffing (special xids connect/ping/auth/setwatches) + opcode dispatch + per-opcode min-length validation + the `max_packet_bytes` check, with partial frames reassembled in a decoder-internal buffer; a decode failure increments `decoder_error` and ABANDONS the buffer (no resync; AMEND-A8). The chain buffer is READ, never drained.
- **The 201-counter roster + creation parity.** All 201 macro counters are created EAGERLY at config-load under scope `<stat_prefix>.zookeeper.` (4 plain + 28 `_rq` + 29 `_rq_bytes` + 28 `_decoder_error` + 28×4 `_resp*`; see the `## Stat-name mapping` 136 → 337 block). Response-side counters exist-at-zero until 28.2 (creation parity, D-P5). The four `enable_*` flags gate INCREMENTS, never creation. Auth requests increment the LAZY dynamic per-scheme counter `<stat_prefix>.zookeeper.auth.<scheme>_rq` (a non-builtin scheme collapses to `auth.unknown_scheme_rq`); there is NO static `auth_rq`.
- **Prometheus rendering is FLAT (AMEND-A4 — no labels).** Reference Envoy applies NO tag extraction to zookeeper stats: `/stats/prometheus` emits `envoy_<stat_prefix>_zookeeper_<counter>{}` (empty label set). envoy-go's NEW `.zookeeper.` INLINE-PREFIX arm at `internal/stats/name.go` flattens `<stat_prefix>.zookeeper.<counter>` (the counter MAY contain dots — the dynamic `auth.<scheme>_rq` family) to `envoy_<stat_prefix>_zookeeper_<counter>` (dot→underscore + `envoy_` prefix, NO label promotion — the ADR-0138 bandwidth_limit shape, NOT a `.rbac.`-style tag-extractor).
- **Shallow-decode leniency departure (envoy-go-LENIENT).** Upstream fully parses every opcode's payload (~1100 lines); envoy-go's Q1 scope is COUNTER parity, so the decoder validates only framing + per-opcode min-length. Consequence: a packet with a valid header but malformed payload counts as `<op>_rq` on envoy-go vs `decoder_error` upstream. The fixture corpus contains no such packets (the departure is invisible to the differential).
- **Dynamic-metadata coverage boundary (AMEND-A9).** Upstream's decoder parses payloads primarily to emit per-message connection-level dynamic metadata; envoy-go emits NONE (no consumer; invisible to the differential; cleared per-message upstream). DEFERRED — a documented coverage boundary, not a silent gap.
- **`access_log` parse-accept-ignore.** The `access_log` field is parse-ACCEPTED then IGNORED (upstream parity — completely unread upstream). The latency fields (`default_latency_threshold`, `latency_threshold_overrides`, `enable_latency_threshold_metrics`) are PARSED + PGV-validated at 28.1 but NOT CONSUMED until 28.2 (the response-side latency counters).
- **The re-iteration guarantee (R8 — cross-side-PROVEN).** EVERY frame of EVERY connection is decoded, regardless of how many socket reads deliver it — the ADR-0221 read seam re-feeds post-terminal-handoff socket reads through the read chain, matching reference Envoy's `FilterManagerImpl::onRead` forever-re-iteration. Without the seam, the chain runtime exits its read loop at handoff and a request-decoding filter sees only the FIRST socket read per connection (the Task-16 `0046` divergence). Proven by the `0046` multi-frame arms (2/3/4) going green cross-side (these arms ARE the re-iteration proof; the R4 deliberate-break liveness protocol is recorded).
- **Stats:** 201 eager macro counters + the dynamic `auth.<scheme>_rq` family (request-side incremented at 28.1; response-side incremented at 28.2 — all 201 exist-at-zero since 28.1, creation parity). Project stat surface **136 → 337** (+201).
- **Differential:** `0046-zookeeper-requests` (7 arms, cross-side `StatsAsserter`, the `TCPSink` backend feeding deterministic ZooKeeper request frames; R4 deliberate-break recorded) + `0047-zookeeper-boot-reject` (the `stat_prefix`-required PARSE-REJECT, symmetric `BootRejectFixture`, `ExpectedBootErrorSubstring() = "stat_prefix"`). The 37th fuzzer (`FuzzZookeeperRequestDecode`) covers the shallow request decoder.

**Response-side semantics (28.2 — cross-side-PROVEN by `0048-zookeeper-responses`; ADR-0223).**

`OnWrite` feeds the response decoder, which reassembles frames in `writeBuf` and dispatches by leading int32 (xid). ALWAYS returns `Continue` (unconditional passthrough — R3 extended to the write side; bytes are never mutated or stopped). A decode failure abandons `writeBuf` (AMEND-A8 symmetry: no resync; connection never closed; subsequent writes decode normally).

**Response dispatch table:**

| Leading int32 | Response type | Minimum frame bytes | Correlation | Counter action |
|---|---|---|---|---|
| `0` (connect) | Connect response | 20 (proto_version+timeout+session_id+password_len — **no zxid/error**) | FIFO-pop `controlRequestsByXid[connectXid]` | `connect_resp` + `response_bytes` (+ flag-gated `connect_resp_bytes`) + latency |
| `−1` (watch) | Watch-event push | **28** (xid(4)+zxid(8)+error(4)+event_type(4)+client_state(4)+path-len(4)) | NEVER correlated | `watch_event` + `response_bytes` only; **no fast/slow** |
| `−2/−4/−8` | Control response (ping/auth/setwatches) | 16 (standard xid+zxid+error ReplyHeader) | FIFO-pop `controlRequestsByXid[xid]` | `<op>_resp` + `response_bytes` (+ flag-gated `<op>_resp_bytes`) + latency |
| `> 0` | Data response | 16 (standard xid+zxid+error ReplyHeader) | Erase-on-lookup `requestsByXid[xid]` | `<op>_resp` + `response_bytes` (+ flag-gated `<op>_resp_bytes`) + latency |
| Any other negative | Unknown | — | None | `decoder_error` only |

**Watch-event minimum — D-S28.2-1 CORRECTED at IMPL.** The 28.2 SPEC originally pinned 16 bytes. Upstream `parseWatchEvent` (decoder.cc:1036, v1.37.2) requires `SERVER_HEADER_LENGTH + 3*INT_LENGTH = 16 + 12 = 28`. Every non-connect response carries the FULL ReplyHeader (xid+zxid+error) before the event payload — the 16-byte SPEC pin omitted zxid(8)+error(4) and was corrected to **28 bytes** at IMPL.

**Correlation-consumption semantics:**

- **Data map (erase-on-lookup):** a second response with the same xid finds nothing → `decoder_error`.
- **Control FIFO queues:** ping/auth/setwatches/connect responses FIFO-pop their per-xid queue; empty queue → `decoder_error`.
- **Correlate-then-validate order (upstream parity):** the pop/erase happens BEFORE the minimum-length check — a malformed-but-correlatable response consumes the pending entry and fires the flag-gated `<op>_decoder_error`.
- **The `connect_readonly` → `connect` response-opname mapping:** `respOpnames` has NO `connect_readonly_resp`; popped entries with opname `"connect_readonly"` are mapped to `"connect"` before any counter call (`respOpname()` + the hardcoded "connect" in `onConnectResponse`).
- **Correlation structures now drained by responses (upstream parity):** data map erased per data response; control FIFO popped per control/connect response. The 28.1 boundary "control queues grow unbounded for the connection's lifetime" is REWRITTEN — responses drain both structures at 28.2. Residual unanswered-request growth (requests with no server response) is upstream's behavior too.

**Latency-threshold semantics (flag-gated; ADR-0223 §Decision item 5):**

- `enableLatencyThresholdMetrics` gates all fast/slow increments (not creation — AMEND-A2). Watch events and decoder errors never receive fast/slow (no correlation hit → no request timestamp).
- Threshold = the wire-opcode-keyed override from `latencyThresholdOverrides` (keyed by WIRE opcode int32, built at 28.1 config parse via `protoToWireOpcode`); fallback to `defaultLatencyThreshold`.
- Comparison is **INCLUSIVE**: `latency <= threshold` → `<op>_resp_fast`; otherwise `<op>_resp_slow` (AMEND-A10).
- Deterministic differential discipline: `default_latency_threshold: 3600s` arm makes ALL responses fast; `default_latency_threshold: 0.001s` + `TCPZKResponder` fixed ≥10 ms delay makes ALL responses slow; a wire-opcode-keyed override (`GetData: 3600s`) on the slow listener makes `getdata_resp_fast` +1 while other ops are slow (proving override-beats-default).

**Proto enum vs wire-opcode enum (AMEND-A6):** `latency_threshold_overrides` config is keyed by the **27-value proto `Opcode` enum** (contiguous values 0..26); `protoToWireOpcode` maps each proto value to its wire int32 in the **26-value gapped wire-opcode enum** (negative `opClose = -11`, gaps, `>100` values). The runtime map is keyed by wire int32; the decoder uses `entry.wireOpcode` (from the correlation entry, set at request-decode time) as the map key. The two enums differ by design — a reviewer encountering both should expect the mismatch.

**Response-side shallow-decode leniency departure (§2.2):** a response with a valid header but malformed payload counts `<op>_resp` on envoy-go vs `decoder_error` upstream (the fixture corpus contains no such frames). Additionally, the universal pre-dispatch minimum is 4 bytes (our xid-only sniff) vs upstream's 16 (xid+zxid+error before correlation) — observable only for 4–15-byte degenerate frames no real server produces: our decoder correlates-then-errors; upstream errors without correlating.

**Latency-HISTOGRAM coverage boundary (ADR-0060 — unmirrored):** `connect_response_latency` / `<opname>_latency` / `unknown_opcode_latency` are NOT emitted. The fast/slow threshold counters are the deterministic stand-in. Deferred under the project-wide histogram deferral.

- **Differential:** `0048-zookeeper-responses` (4 listeners / 8 arms; cross-side `StatsAsserter`; `TCPZKResponder` BackendKind = 29 with fixed ≥10 ms delay; R4 deliberate-break + R5 correlation ratification recorded; GREEN on first run). The 38th fuzzer (`FuzzZookeeperResponseDecode`) covers the response-decoder path (30 s / 1,809,632 execs, zero crashers).

### envoy.filters.network.mongo_proxy

*Introduced by phase 29.1 (`network-filter-mongo-wire-and-requests`) — the FOURTH §9 Network-filters-family row (a flat top-level row per ADR-0106) + the family's second stats-PRIMARY filter + consumer #2 of the ADR-0221 `network.WriteFilter` conn-wrap seam (exactly as that ADR's §Consequences anticipated). Justified by ADR-0224 (the `mongo_proxy` filter, request side). Cross-side-PROVEN by the now-green `0049-mongo-requests` fixture. The response side (OP_REPLY/OP_COMMANDREPLY decode + correlation + the gauge increments) lands at 29.2 (ADR-0225); fault-delay + access-log + drain land at 29.3 (ADR-0226).*

`mongo_proxy` (`@type` `type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` — type URL DERIVED via `proto.MessageName`, NOT hand-typed; the `extensions.` segment is intrinsic) is a passive observability sniffer that decodes the MongoDB legacy wire protocol (little-endian MsgHeader + BSON) flowing through a `[mongo_proxy, tcp_proxy]` chain and emits per-opcode / per-command / per-collection counters plus the `op_query_active` gauge. It NEVER mutates bytes, never halts iteration (no fault delay at 29.1 — that is 29.3), never closes — its entire operator-visible surface IS its stats (the body differential is intrinsically vacuous; the cross-side label-aware `StatsAsserter` is the load-bearing proof). It is the 8th built-in network filter and implements BOTH `ReadFilter` and `WriteFilter` as ONE instance (consumer #2 of the ADR-0221 seam — the no-op `OnWrite` stub is what qualifies the chain for the 28.1b read seam).

- **Request-side decode semantics (§3.5).** `OnNewConnection` is a no-op `Continue` (an OnNewConnection `StopIteration` would set the sticky `connHalted` flag — `reference_network_read_filter_onnewconnection_halts`). `OnData` feeds the decoder with the chain buffer's NEW bytes (tracked by the `chainConsumed` high-water mark against `Buffer.TotalAppended()` — the 28.1b §3.3 mechanism adapted verbatim, D-S29.1-4) and ALWAYS returns `Continue` (unconditional passthrough; R3 — never drains/mutates/halts/closes; the decoder COPIES the chain bytes into its private `readBuf`, never draining the chain buffer). `OnWrite` is a PURE no-op `Continue` at 29.1 (it does NOT buffer write-direction bytes — there is no response decoder to drain them until 29.2, so buffering would grow unbounded on long-lived connections; the stub exists so the filter satisfies `WriteFilter` end-to-end and the `0049` traffic exercises the `writeChainConn → OnWrite` seam). The decoder does 16-byte LE MsgHeader framing (`messageLength` INCLUDES the header) + opcode dispatch + per-opcode body decode, with partial frames reassembled in the private `readBuf` (a trailing partial message is NEVER an error — upstream parity).
- **The EXACTLY-7-opcode decode envelope + OP_MSG-not-decoded (UPSTREAM PARITY, not a gap; AMEND-B5).** Upstream v1.37.2's codec dispatch decodes exactly Reply(1), Insert(2002), Query(2004), GetMore(2005), KillCursors(2007), Command(2010), CommandReply(2011). At 29.1: Query/Insert/GetMore/KillCursors/Command are body-decoded; Reply(1)/CommandReply(2011) are RECOGNIZED-NOT-DECODED (a valid envelope → NOT a `decoding_error`; the frame is consumed; their body decode + counters land at 29.2). The modern OP_MSG(2013), the legacy Msg(1000), Update(2001), and Delete(2006) all take the `decoding_error` path (upstream `EnvoyException("invalid mongo op N")` parity). Per `reference_wire_format_both_sides_see_same_bytes`, envoy-go mirrors EXACTLY this envelope — OP_MSG-not-decoded is upstream's behavior, NOT an envoy-go gap.
- **Sniffing-off-on-error connection-lifetime semantics (AMEND-B6; D-S29.1-6).** Any decode error (unknown opcode, BSON error, dot-free collection name, empty command doc, buffer underflow inside a complete frame) increments `decoding_error` **at most ONCE per connection** (the increment + the flag-set are atomic) AND sets `sniffing = false` — decode permanently STOPS for the connection. Subsequent `OnData`/`OnWrite` calls advance the high-water mark and DROP the bytes without decoding (the private buffer is released). Passthrough is unaffected; the connection is NEVER closed; `OnData` still returns `Continue`. This is structurally DIFFERENT from zookeeper's abandon-buffer-keep-decoding model (pinned + unit-tested + fuzz-checked). Cross-side-proven by `0049` arm 6: the first decode error turns sniffing off, so a follow-up VALID OP_QUERY on the SAME connection increments NOTHING (the dropped-query proof — `op_query{mongo_a}` stays at 5, not 6).
- **The 23-stat EAGER roster + the boot-window creation departure (D-P1).** All 23 fixed stats (22 counters + the `op_query_active` gauge) are created EAGERLY at config parse under scope `mongo.<stat_prefix>.` (via `NewCounterIfAbsent`/`NewGaugeIfAbsent` — post-Freeze-permitted, idempotent across listeners sharing a prefix). The roster is normatively defined by `internal/filter/network/mongoproxy/stats.go::rosterSuffixes()` + the R2 golden test `TestStatRoster_MatchesUpstreamMacro` (a sorted golden list transcribed from upstream's `ALL_MONGO_PROXY_STATS` macro; `delays_injected` is PLURAL — the regression guard). **Boot-window creation departure (envoy-go-strict, D-P1):** upstream creates the fixed roster per-connection (pool-deduped) — the reference admin shows nothing until the first downstream connection. envoy-go's EAGER posture means the counters exist-at-zero from config-load. This is UNOBSERVABLE to the differential (every `0049` assertion runs post-first-connection) and is recorded here as a coverage boundary, not a silent gap. The eager posture also guarantees the 29.2/29.3 response/fault increments cannot regress creation (the zookeeper D-P5 precedent).
- **The `mongo.<stat_prefix>.` scope (AMEND-B1).** The literal `mongo.` root is confirmed live; the 22-counter roster carries ZERO macro histograms (all mongo histograms are dynamic-family members, deferred per ADR-0060). The gauge is `mongo.<stat_prefix>.op_query_active`.
- **Prometheus exposition is TAG-EXTRACTED — the four-rule label hoisting (§7.4; AMEND-B2 + AMEND-C1; the INVERSE of the phase-28 zookeeper finding).** Reference Envoy applies FOUR `addTokenized` tag-extraction rules to mongo stats (upstream `well_known_names.cc:86-98,180-181`). envoy-go's NEW `mongo.` arm at `internal/stats/name.go` (the `.rbac.` ADR-0218 label-promotion precedent generalized to MULTI-label) mirrors them exactly:

| Internal name shape | Prometheus name | Labels (sorted by key) |
|---|---|---|
| `mongo.<sp>.<fixed>` (the 23 fixed stats) | `envoy_mongo_<fixed flattened>` | `envoy_mongo_prefix="<sp>"` |
| `mongo.<sp>.cmd.<cmd>.total` | `envoy_mongo_cmd_total` | `envoy_mongo_prefix` + `envoy_mongo_cmd="<cmd>"` |
| `mongo.<sp>.collection.<c>.query.<leaf>` | `envoy_mongo_collection_query_<leaf>` | `envoy_mongo_prefix` + `envoy_mongo_collection="<c>"` |
| `mongo.<sp>.collection.<c>.callsite.<cs>.query.<leaf>` | `envoy_mongo_collection_callsite_query_<leaf>` | `envoy_mongo_prefix` + `envoy_mongo_collection="<c>"` + `envoy_mongo_callsite="<cs>"` |

  The dynamic segment values (cmd / collection / callsite) NEVER appear in the Prometheus metric name (AMEND-C1 — distinct commands/collections/callsites collapse onto the same `# TYPE` family, differentiated by labels; this REFUTES the parent D-P2 anticipation that "dynamic segments stay in the name"). The gauge emits `# TYPE envoy_mongo_op_query_active gauge`. The arm SORTS its labels LOCALLY by key (matching the reference's alphabetical order) → **`prom.go` stays BYTE-UNTOUCHED (D-S29.1-5)**; `name.go` is the ONLY `internal/stats/` file changed.
- **The dynamic cmd/collection/callsite counter families + commands-remembering + alias normalization.** The `cmd.<cmd>.total`, `collection.<c>.query.{total,scatter_get,multi_get}`, and `collection.<c>.callsite.<cs>.query.*` families are created LAZILY at decode time (post-Freeze-permitted is REQUIRED here — the zookeeper `auth.<scheme>_rq` / rbac per-policy precedent) and are NOT counted in the static 360 surface (config/traffic-dependent). Command detection: a fullCollectionName containing `$cmd` routes to the command path — the command name is the first element key of the (possibly `$query`-nested) command document, run through alias normalization (§3.3) then the remembered-set (`commands` config, default `{delete,insert,update}`); an unlisted command folds into `cmd.unknown_command.total` (AMEND-B7). `find` commands route to the QUERY path (collection stats). Query-shape heuristics (non-command queries): no `_id` → ScatterGet; Document/Array `_id` → MultiGet; scalar `_id` → PrimaryKey (only `.query.total`). Every non-command query increments `op_query` + `collection.<c>.query.total` + appends to the active-query list.
- **The `stats.IsValidName` guard divergence (envoy-go-strict coverage boundary; USER-APPROVED — D-S29.1 IMPL deviation).** The three dynamic-stat builders (`cmdTotal`/`collectionQuery`/`callsiteQuery`) feed WIRE-DERIVED segments (command names, collection names, callsite strings) into `NewCounterIfAbsent`, which PANICS on names failing the stats `nameRE` charset. envoy-go therefore guards each dynamic increment with `stats.IsValidName` and SKIPS the dynamic stat for an un-nameable wire-derived collection/callsite/command segment that upstream WOULD emit (upstream's stat-name handling tolerates a broader charset). The fixed counters + the active-query append ALWAYS run regardless. **This is a recorded coverage boundary, not a silent gap:** the differential is UNAFFECTED (all `0049`/`0050` fixtures use nameable identifier segments); the no-panic guarantee is FUZZ-VALIDATED (`FuzzMongoDecode`, 60 s / 16 M execs, no `stats: invalid metric name` panic against un-nameable wire bytes). Live guard test: `TestDecodeQuery_InvalidCollectionNameSkipsDynamicNoPanic` (proven live: RED panic without the guard → GREEN with it).
- **The `cx_destroy_*_with_active_rq` presence-only boundary (AMEND-C2).** `cx_destroy_local_with_active_rq` / `cx_destroy_remote_with_active_rq` increment on the REFERENCE during 29.1 fixtures (unanswered queries at connection close), but envoy-go does NOT wire their increments until 29.2 (the close-direction-keyed teardown is correlation-coupled — parent D-P4). The `0049` assertions for this pair are therefore PRESENCE-ONLY (the counters exist at 0 both sides; value parity lands at the 29.2 `0051` fixture). Arm 8 asserts presence without comparing values.
- **The OP_REPLY/OP_COMMANDREPLY recognized-not-decoded 29.1 boundary (§1.2 — a SUB-PHASE boundary, not a permanent departure).** Reply(1)/CommandReply(2011) are recognized as valid envelopes (consumed, NOT a `decoding_error`) but their bodies are not decoded and `op_reply*`/`op_command_reply` do not increment until 29.2. The response-side counters (`op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`, `op_command_reply`) exist-at-zero from config-load (creation parity, D-P1) and are increment-wired at 29.2. This is a sub-phase boundary closed at 29.2, not an envoy-go departure.
- **Runtime-key gating unmirrored (envoy-go-strict; 29.1 subset — §7.5).** Sniffing is always on (the upstream `mongo.proxy_enabled` runtime key default); the fault/logging runtime keys are recorded fully at 29.3. The runtime-keys-at-defaults posture is the 29.1-landing subset of the parent §7.5 departure roster.
- **Stats:** 23 eager fixed stats (22 counters + the `op_query_active` gauge) + the dynamic `cmd.*`/`collection.*`/callsite families. Request-side counters increment at 29.1; the response-side `op_reply*`/`op_command_reply` and the gauge increments land at 29.2; `cx_drain_close` + `delays_injected` at 29.3. Project stat surface **337 → 360** (+23; see the `## Stat-name mapping` 337 → 360 block).
- **Differential:** `0049-mongo-requests` (9 arms; two listeners `l_default`/mongo_a + `l_commands`/mongo_b; cross-side label-aware `StatsAsserter`; the EXISTING `TCPSink` backend BackendKind = 28 per AMEND-C4 — NO new BackendKind at 29.1; the gauge `# TYPE … gauge` line asserted present; `cx_destroy_*` presence-only per AMEND-C2; the `$comment` callsite AMEND-C3 double-count; R4 deliberate-break recorded with `-count=1`) + `0050-mongo-boot-reject` (the `stat_prefix`-required PARSE-REJECT, symmetric `BootRejectFixture`; reference PGV `MongoProxyValidationError.StatPrefix`; envoy-go `mongo_proxy: stat_prefix is required`). The 39th fuzzer (`FuzzMongoDecode`) covers the shallow request decoder (no-panic + chain-bytes-unmutated R3 + sniffing-off idempotence AMEND-B6 + bounded readBuf).
- **Forward-pointers.** **29.2 (ADR-0225):** the OP_REPLY/OP_COMMANDREPLY response decoder in `OnWrite`; requestID↔responseTo correlation consuming the 29.1 active-query list (first-match-erase; the per-connection ADR-0223 mutex with the cross-goroutine `OnWrite`-pump-B reader); the `op_query_active` gauge inc/dec (the project's first differentially-mirrored gauge); `cx_destroy_*_with_active_rq` value parity; `emit_dynamic_metadata` (the ADR-0217 Bucket third production write); fixture `0051-mongo-responses` + the `TCPMongoResponder` BackendKind (anticipated value 30). **29.3 (ADR-0226):** the async halt/resume seam; fault-delay injection (`delays_injected`); the mongo access log; `cx_drain_close`; fixture `0052-mongo-fault-delay`; the parent-row-29 ROLLUP.

### tcp_proxy per-connection cluster resolution — 27 amendment

*Amended by phase 27 (`network-filter-sni-cluster`). Justified by ADR-0219 (the connection-scoped upstream-cluster-override seam). NO operator-visible behavior change to `tcp_proxy` when no override is published — the no-override path is byte-exact with master tip (the existing `tcp_proxy` fixtures `0000`/`0001`/`0002` + the 26.x network fixtures stay green — the regression gate).*

At master tip `tcp_proxy` resolved its upstream cluster ONCE at boot (the configured `cluster:` → a stored `*cluster.Cluster`). Phase 27 makes the resolution per-connection: the `Filter` retains a `*cluster.Manager` + the boot-resolved `defaultCluster` (the configured cluster). At `Handle` (before the Dial), `tcp_proxy` resolves the effective cluster:

- **Override present & non-empty** → `cm.Get(override)`: found → that cluster (even if it equals the configured name — `cm.Get` resolves identically, no special case); MISS (unknown cluster) → close the downstream connection with ZERO application bytes (`log.Printf("tcpproxy: per-connection override cluster %q not found", override)` + `return`). This mirrors Envoy's unknown-override path behaviorally (`getThreadLocalCluster` null → `downstream_cx_no_route` + `NoFlush` close — the downstream sees a zero-byte close; F-NOROUTE / D27-4).
- **Override absent / empty** → `defaultCluster` → byte-exact with master tip (F-RESOLVE).

The override is the envoy-go stand-in for Envoy's `PerConnectionCluster` filter-state entry (key `envoy.tcp_proxy.cluster`), threaded out-of-band on the call `ctx` (`network.UpstreamClusterOverride(ctx)`) so the `TerminalFilter.Handle(ctx, conn)` signature (shared by HCM) is UNCHANGED. The `NewFilter` boot-time parse + the byte-stable `tcpproxy: cluster %q not found` / `weighted_clusters` rejects are UNCHANGED. `weighted_clusters` is moot — envoy-go rejects it at parse, keeping it on the single-cluster path where Envoy honors the override (F-WEIGHTED).

**Coverage boundary — `downstream_cx_no_route` (and the wider `downstream_cx_*` family) is a known-unmirrored upstream counter.** Envoy increments `tcp.<stat_prefix>.downstream_cx_no_route` on the unknown-override path (F-NOROUTE / D27-4). envoy-go's `tcp_proxy` emits NO `downstream_cx_*` counters at all today (a pre-existing gap, NOT introduced by phase 27); adding a single `downstream_cx_no_route` only on the override-miss path would be an inconsistent partial of an absent family. Phase 27 DEFERS the whole `downstream_cx_*` family (+0) and records `downstream_cx_no_route` as a known-unmirrored upstream counter. The unknown-override behavior (downstream close / zero bytes) IS proven cross-side byte-exact (fixture `0045-sni-cluster` arm 3); the stat absence is invisible to a response-body differential. The narrow typed override is the envoy-go stand-in for Envoy's `envoy.tcp_proxy.cluster` filter-state key — there is NO general connection-scoped filter-state primitive (Q2/YAGNI; it generalizes if/when a SECOND override-publishing consumer appears — API-revision allowance). Project stat surface stays **136** (+0).

### Network filter chain framework — terminal-handoff conn-wrap seam (28.1 amendment)

*Introduced by phase 28.1 (write half [28.1a] + read half [28.1b]). Justified by ADR-0221 (the terminal-handoff conn-wrap seam, both directions) — CONSUMING the ADR-0213 §Decision-item-8 API-revision allowance (consumer #1 `zookeeper_proxy`; anticipated #2 `mongo_proxy`). NO operator-visible behavior change to any existing chain: every chain with ZERO write filters gets NEITHER conn-wrap → `handleTerminal` is byte-identical to 26.2/27 (R1, ratified by all 47 pre-existing fixture dirs staying byte-exact green).*

The L4 chain framework gains a symmetric write + read seam at terminal handoff. `NewChainRuntime` classifies a built `[]NetworkFilter` by INDEPENDENT type-asserts into a read set, a write set, and an optional trailing terminal — a filter implementing BOTH (zookeeper_proxy) lands in BOTH sets as the SAME instance (dual callback injection; `OnDestroy` deduped once-per-instance via interface-identity). Both conn-wraps install under ONE shared predicate (`len(writeFilters) > 0`); a chain gets BOTH wraps or NEITHER. The terminal sees `writeChainConn(prefixConn(readChainConn(conn)))` (readChainConn innermost, writeChainConn outermost).

**Write side (28.1a):**

- **REVERSE-chain-order write dispatch (AMEND-A11 LIFO).** `handleTerminal` hands the `writeChainConn` a REVERSED copy of the chain-order write filters: config `[A, B, C]` ⇒ write dispatch `C → B → A` (upstream `addWriteFilter` front-insert / LIFO parity). The `writeChainConn.Write(p)` runs the dispatch-order chain over a fresh per-Write `*Buffer`, then forwards the post-chain buffer to the wrapped conn.
- **`StopIteration`-on-write = the-write-does-not-proceed, documented-UNSUPPORTED-by-consumers.** envoy-go mirrors the upstream no-forward semantic (the bytes never reach the transport); the return is `(len(p), nil)` (the terminal cannot distinguish a stopped write from a delivered one — D-P7, exactly as upstream `ConnectionImpl::write` returns void). No production filter returns it at 28.x (zookeeper always Continues); the no-resume/inject machinery is deferred to the first consumer (mongo fault-delay) under the API-revision allowance.
- **Terminal-originated-writes-only.** A `ReadFilter` writing via `Connection().Write` (echo, direct_response) does NOT pass through the write chain — the `writeChainConn` wraps only the conn handed to the TERMINAL. Sufficient for `[zookeeper_proxy, tcp_proxy]` (all upstream→downstream bytes are produced by tcp_proxy).
- **The write-only-filter boot boundary.** `internal/listener/manager.go` is UNTOUCHED by the seam; a write-ONLY filter (no `ReadFilter` half) leading a chain still fails boot exactly as before. Every 28.x both-directions filter carries a `ReadFilter` half, so this constrains nothing real (lifted under the allowance if a write-only consumer appears).

**Read side (28.1b):**

- **Replay semantics (§3.2).** The `readChainConn.Read` reads the live socket, then `chainRuntime.replayRead` re-iterates EVERY read filter's `OnData` in chain order over the received bytes BEFORE returning them to the terminal (replay-before-return → deterministic stat ordering), then DRAINS the chain buffer fully after the pass (bounded memory). On `io.EOF` a final endStream replay fires (pre-handoff read-loop symmetry). This restores upstream `FilterManagerImpl::onRead` re-iteration parity — without it the chain runtime exits its read loop at handoff and a request-decoding filter sees only the FIRST socket read per connection (the gap that forced the ADR-0045 28.1a/28.1b split). R3 is preserved: filters never drain — the RUNTIME drains.
- **The shared wrap predicate + R1 (§3.4).** Both conn-wraps install IFF `len(writeFilters) > 0`. Equivalent TODAY to "≥1 read-decoding filter" (every planned request-decoding network filter is a both-directions WriteFilter). A hypothetical one-direction read-decoder would miss this predicate and generalize via an opt-in marker interface under the allowance — none exists or is planned; the next consumer `mongo_proxy` must implement `WriteFilter` (even as a no-op) to get the read seam. Hot-path cost is wrapped-chains-only (one indirection + one Append + one read-chain pass + one Drain per socket read); the R1 population pays ZERO.
- **The `TotalAppended` soundness invariant (§3.3).** The decoder's high-water mark is kept against `Buffer.TotalAppended()` (a monotonic int64 appended-bytes counter), NOT the physical buffer length, so the decoder feed is correct regardless of WHO drains the buffer and WHEN. SOUND iff the runtime never drains bytes a filter has not yet been shown — both drain sites (the handoff drain; the post-replay drain) satisfy it by construction. On any never-drained execution `TotalAppended() == Len()`, so the pre-handoff path is byte-identical to 28.1a.
- **The three observational post-handoff boundaries (§3.5).** The replay is OBSERVATIONAL; three pre-handoff callback capabilities have no post-handoff effect (each a documented framework boundary, deferred to the first consumer needing it under the allowance — no production consumer needs any: zookeeper always Continues, never closes, never drains):

| Capability | Post-handoff (the replay) | Why acceptable |
|---|---|---|
| `OnData` → `StopIteration` | **IGNORED** — the bytes are already committed to the terminal (readChainConn returns them regardless) | Upstream divergence (upstream StopIteration blocks tcp_proxy from seeing the bytes). Honoring it would require the seam to suppress the terminal's read — a flow-control surface no 28.x consumer needs. |
| `Connection().Close(...)` | **NOT ACTED ON** — recorded but nothing post-handoff checks it (the terminal owns the conn lifecycle) | No production consumer closes post-handoff; a kill-on-protocol-violation policy would need a terminal-integration surface (deferred). |
| `ContinueReading()` | **MEANINGLESS** — no parked state exists post-handoff (Status is ignored) | Follows from row 1. |

- **The goroutine-placement note + the per-connection decoder mutex (§3.6 — 28.2 LANDED; ADR-0223).** Post-handoff, two pump goroutines run: goroutine A (downstream→upstream) calls `readChainConn.Read → replayRead →` the request decoder; goroutine B (upstream→downstream) calls `writeChainConn.Write →` the write filters' `OnWrite`. At 28.1b they shared NO mutable state (B's `OnWrite` was a no-op; `writeChainConn.Write` allocates a fresh per-Write `Buffer`) — **the 28.1b race surface was EMPTY** (verified `go test -race` + a dedicated concurrent-pumps unit test over `net.Pipe`). **FORWARD-POINTER DISCHARGED (ADR-0223 §Decision item 4):** the 28.2 response decoder lands in `OnWrite`, so goroutine B now READs + ERASEs the correlation structures goroutine A WRITEs. The race is closed by a **per-connection `sync.Mutex mu`** on the decoder guarding EXACTLY the two correlation maps (`requestsByXid` + `controlRequestsByXid`) — nothing else. The reassembly buffers (`readBuf` / `writeBuf`) and `chainConsumed` remain single-goroutine-owned and lock-free. Entries are COPIED OUT under the lock; counter increments + latency math run outside it. The pre-handoff request path locks too (uniformity; uncontended pre-handoff). `OnDestroy` needs no lock (runs strictly after both pumps join — the `wg.Wait()` happens-after edge). Proven by `TestDecoderConcurrentRequestResponseRace` (`-race -count=5`; removing `mu` triggers an immediate race report) + the live `0048` concurrent pumps (R9 ratified). Upstream has no such race only because libevent serializes both directions onto one dispatcher thread.

### Type-URL correction (echo `@type`)

The phase-26.1 SPEC §3.6/§4.1 originally pinned echo's type URL as `type.googleapis.com/envoy.filters.network.echo.v3.Echo`. The ACTUAL go-control-plane v1.32.4 proto full-name carries the `extensions.` segment: **`type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo`** (verified at 26.1 IMPL via `proto.MessageName`, the boot smoke, and fixture `0040` against real upstream Envoy v1.37.2 — without `extensions.` the binary cannot boot an echo config: `protojson: ... "not found"`). `echo.TypeURL` was corrected to the `extensions.` form (and the SPEC §3.6/§4.1 lines corrected in-place at 26.1 IMPL Task 17). `direct_response`'s type URL (`...envoy.extensions.filters.network.direct_response.v3.Config`) already carried `extensions.` and was correct.

### Stat surface

`echo`: 0 built-in stats. `direct_response`: 0 built-in stats. The framework adds 0 counters. The 26.2 `tcp_proxy`/HCM migration adds 0 counters (their existing stats are untouched). Project stat surface was **132** at 26.1 AND 26.2 phase-done; the phase-26.3 `rbac_network` 4-counter roster (`<stat_prefix>.rbac.{allowed,denied,shadow_allowed,shadow_denied}`) lands **132 → 136** (see the rbac_network stat-table extension block in `## Stat-name mapping`). Phase 26.3 also adds 2 fixture dirs (44 → 46) + 1 fuzzer (35 → 36). Phase 27 `sni_cluster` adds **0** counters (config-less; upstream parity) — stat surface stays **136** (+0); the `tcp_proxy` `downstream_cx_*` family stays unmirrored (a pre-existing gap; see the `tcp_proxy per-connection cluster resolution — 27 amendment` above). Phase 27 adds 1 fixture dir (46 → 47, `0045-sni-cluster`) + 0 fuzzers (36, DEFERRED — echo-parity config-less parse). Phase 28.1 `zookeeper_proxy` adds the **201**-counter eager roster (`<stat_prefix>.zookeeper.` + the dynamic `auth.<scheme>_rq` family) — stat surface **136 → 337** (the largest single-filter addition; the roll lands at 28.1b on the cross-side proof, not 28.1a; see the `### envoy.filters.network.zookeeper_proxy` subsection + the `## Stat-name mapping` 136 → 337 block). Phase 28.1 adds 2 fixture dirs (47 → 49: `0046-zookeeper-requests` re-enabled + `0047-zookeeper-boot-reject`) + 1 fuzzer (36 → 37, `FuzzZookeeperRequestDecode`). The ADR-0221 terminal-handoff conn-wrap seam (both directions) adds **0** stats of its own. Phase 28.2 adds **0** new counters (zero creation delta — all 201 `zookeeper_proxy` counters were created at 28.1; 28.2 wires the response-side increments only; stat surface stays **337**); adds 1 fixture dir (49 → 50: `0048-zookeeper-responses`) + 1 fuzzer (37 → 38: `FuzzZookeeperResponseDecode`). The THIRD §9 Network-filters-family row closes at 28.2; 4 family candidates remain (`redis`/`mongo`/`kafka_broker`/`thrift`); `mongo_proxy` is the natural next + anticipated consumer #2 of the ADR-0221 conn-wrap seam. Phase 29.1 `mongo_proxy` (request side) adds the **23**-stat eager roster (22 counters + the `op_query_active` gauge under `mongo.<stat_prefix>.` + the dynamic `cmd.*`/`collection.*`/callsite families) — stat surface **337 → 360** (the FOURTH §9 family row + the family's second stats-PRIMARY filter + consumer #2 of the ADR-0221 conn-wrap seam; see the `### envoy.filters.network.mongo_proxy` subsection + the `## Stat-name mapping` 337 → 360 block). Phase 29.1 adds 2 fixture dirs (50 → 52: `0049-mongo-requests` + `0050-mongo-boot-reject`) + 1 fuzzer (38 → 39, `FuzzMongoDecode`). Prometheus rendering is TAG-EXTRACTED (the four-rule `mongo.` arm at `internal/stats/name.go`; AMEND-B2 + AMEND-C1 — the inverse of the zookeeper FLAT finding). The response side (`op_reply*`/`op_command_reply` increments + the gauge inc/dec + `cx_destroy_*` value parity) lands at 29.2 (+0 creation); fault-delay + access-log + drain land at 29.3 (+0 creation).

### Forward-pointer note (26.3)

- **26.2 (DONE)** migrated `tcp_proxy` (`internal/filter/tcpproxy/`) + HCM (`internal/filter/hcm/`) onto the framework + the extensible `*network.Registry` via the `TerminalFilter` seam, RETIRED the hardcoded `filterRegistry` map + `filterHandler`/`filterConstructor` types + `buildTerminalFilter` + `listenerCtx` in `internal/listener/manager.go`, unified the dispatch (the `network-filter-mixed-chain-unsupported` transitional reject lifted; the NEW `network-filter-terminal-not-last` / `network-filter-multiple-terminals` chain-shape rejects added), and proved the framework on the load-bearing path (back-compat via the existing fixtures + conformance + h2spec staying byte-exact green). See the 26.2 framework-update block above.
- **26.3 (DONE)** landed `rbac_network` (`envoy.extensions.filters.network.rbac.v3.RBAC`) at full upstream parity (enforced + shadow rules + connection-level dynamic-metadata emission + the `allowed`/`denied`/`shadow_allowed`/`shadow_denied` stat roster) + extracted the shared `internal/rbac/` engine (HTTP rbac migrated as consumer #1 byte-exact, network rbac as consumer #2) + the first real connection-level `*dynamicmetadata.Bucket` writes + the FIRST production mixed read→terminal chain (`rbac_network` allow → `Continue` → `tcp_proxy`/HCM via the `prefixConn` handover shaped at 26.2) + the deferred "read filter preceding terminal" differential fixture. See the `### envoy.extensions.filters.network.rbac` subsection above. The §9 Network-filters family stays OPEN (6 candidates remain: redis / mongo / kafka_broker / thrift / zookeeper / sni_cluster).

### Applies to
- Phase 26.1 onward (L4 read-filter chain framework + `echo` + `direct_response`).
- Phase 26.2 onward (the `TerminalFilter` connection-takeover seam; `tcp_proxy`/HCM migrated onto the framework + `*network.Registry`; unified single-dispatch path; `[read*, terminal?]` chain shape; hardcoded-registry retirement).
- Phase 26.3 onward (`rbac_network` L4 RBAC filter; the shared `internal/rbac/` engine + input-capability `Profile`; the FIRST connection-scoped `*dynamicmetadata.Bucket` writes; the FIRST production mixed read→terminal chain; the `NoFlush` F3 production touchpoint).
- Phase 28.1 onward (the terminal-handoff conn-wrap seam, both directions — the `WriteFilter`/`WriteFilterCallbacks` interfaces, the both-directions classification + dual injection, the REVERSE-order write dispatch + `writeChainConn`, the `readChainConn` post-handoff replay + the `TotalAppended` feed re-base, the shared `len(writeFilters) > 0` wrap predicate; `zookeeper_proxy` request side — the 201-counter roster + the shallow request decoder + the dynamic auth counters; ADR-0221 + ADR-0222).
- Phase 28.2 onward (`zookeeper_proxy` response side — the response decoder in `OnWrite` (response framing + watch-event handling + connect-response special framing), correlation consumption (data map erase-on-lookup + control FIFO pop), `connect_readonly`→`connect` opname mapping, per-opcode `*_resp`/`response_bytes`/flag-gated `*_resp_bytes` increments, latency-threshold fast/slow counters (`latency <= threshold` inclusive; wire-opcode-keyed overrides; flag-gated), the per-connection decoder mutex (discharging the ADR-0221 §Consequences forward-pointer), the `0048-zookeeper-responses` cross-side proof, the 38th fuzzer; ADR-0223. The THIRD §9 Network-filters-family row closes; parent row 28 → done).

### Does not yet apply to
- The §3.5 post-handoff observational boundaries (post-handoff `OnData` `StopIteration` ignored / `Connection().Close` not acted on / `ContinueReading` meaningless) — documented framework boundaries, each deferred to the first consumer needing it under the ADR-0213/0221 API-revision allowance.
- A true read-filter (incremental `OnData`) `tcp_proxy` (the migration uses the `TerminalFilter` seam — a future phase MAY revisit under the ADR-0213 API-revision allowance).
- Cross-side differential proof of the `rbac_network` `requested_server_name` (SNI) + `authenticated` (mTLS) principal/permission arms — UNIT-TESTED only at 26.3 (D-26.3-4 known coverage boundary; the IP/port arms ARE differential-proven).
- Other Network-family filters (redis, mongo, kafka_broker, thrift) — the §9 Network-filters family is OPEN; `mongo_proxy` is the natural next + the anticipated consumer #2 of the ADR-0221 conn-wrap seam (zookeeperproxy is consumer #1; phase 28 closes; 4 candidates remain).
- `zookeeper_proxy` latency HISTOGRAM family (`connect_response_latency` / `<opname>_latency` / `unknown_opcode_latency`) — DEFERRED per ADR-0060 (the project-wide histogram deferral); the fast/slow threshold counters are the deterministic stand-in. A documented coverage boundary, not a silent gap.

---

## HTTPFilterCallbacks

*Introduced by phase 16. Justified by ADR-0144 (TLS-principal accessor framework primitive).*

This section codifies envoy-go-side extensions to the `DecoderFilterCallbacks` + `EncoderFilterCallbacks` interfaces introduced by §9 family-row phases. Each subsection names the accessor / primitive, the phase that introduced it, the justifying ADR, and the cross-phase reuse intent.

### DownstreamPrincipal accessor (per phase 16 ADR-0144)

`DecoderFilterCallbacks.DownstreamPrincipal() []string` returns the priority-ordered list of TLS principal-name candidates for the active downstream client connection:

1. URI SAN values from `tls.ConnectionState.PeerCertificates[0].URIs[].String()` (priority 1 per `rbac.pb.go:1432-1438` proto comment).
2. DNS SAN values from `tls.ConnectionState.PeerCertificates[0].DNSNames` (priority 2).
3. Subject DN Common Name from `tls.ConnectionState.PeerCertificates[0].Subject.CommonName` (priority 3 fallback).

For plaintext or non-mTLS connections OR connections where no client cert was presented, the accessor returns `nil` (or empty slice).

Cross-phase reuse: future filters (jwt_authn, ext_authz, oauth2) consume the same accessor for TLS-principal introspection.

Phase 16 is the FIRST §9 family-row to introduce this accessor.

### 6 new accessors (per phase 18.2 ADR-0165 — the ADR-0044 escape-valve fired at planner-time D3 + D12)

**AMENDMENT to 18.1-anchored claim — `### envoy.filters.http.ext_authz` §13.5 originally pinned "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; that claim is FLIPPED at phase-18.2 PLAN time per planner-time decisions D3 + D12 + IMPL Task 4 landing.** The PLAN-time deviation rationale: at PLAN time the planner re-verified the master-tip callback surface against the 18.2 SPEC's own §15 acceptance item 4 + §11.P4 RATIFICATION (populated `tls_session.sni` / `source.certificate` / source + destination socket addresses / `destination.principal` / `request.http.protocol`) and determined the populated set is UNSATISFIABLE without callback extension — the existing `DownstreamPrincipal()` alone covers only the client-cert principal candidates. The PLAN settled by AMENDING SPEC §13.5 + §6.5 step 5 + §6.6 in-place at Task 4. 6 new methods land on `DecoderFilterCallbacks` at IMPL Task 4 per ADR-0165:

1. **`DownstreamRemoteAddr() net.Addr`** — downstream client remote address (IP + port). Populates `AttributeContext.source.address.socket_address`.
2. **`DownstreamLocalAddr() net.Addr`** — local listener bind address (IP + port). Populates `AttributeContext.destination.address.socket_address`.
3. **`DownstreamTLSServerName() string`** — downstream TLS `ConnectionState.ServerName` (SNI value sent by the client). Populates `AttributeContext.tls_session.sni` when `include_tls_session: true` AND TLS connection. Empty for plaintext.
4. **`DownstreamTLSPeerCertDER() []byte`** — DER-encoded leaf cert of the downstream client (presented during mTLS handshake). Populates `AttributeContext.source.certificate` when `include_peer_certificate: true` AND the client presented a cert. Nil for plaintext / non-mTLS / no-client-cert.
5. **`DownstreamProtocol() string`** — protocol used on the downstream connection (`HTTP/1.1`, `HTTP/2`). Populates `AttributeContext.request.http.protocol`.
6. **`ListenerPrincipal() string`** — listener TLS cert-derived principal (URI SAN | DNS SAN | Subject DN CN, joined per the ADR-0144 priority order applied to the SERVER cert, not the client cert). Populates `AttributeContext.destination.principal` AUTOMATICALLY (NOT gated by `include_peer_certificate` — per the §11.P4 in-session SPEC RATIFICATION). Empty for plaintext listeners.

Plumbing pattern mirrors ADR-0144's `tlsPrincipals` / `SetTLSPrincipals` / `DownstreamPrincipal()` seed-at-HCM-dispatch discipline. The 6 fields are captured at `DecodeHeaders` time in `dispatchOutboundCheck` (per 18.2 IMPL Task 8) and threaded into the extended `*authRequest` struct; the `buildAttributeContext` builder consumes the `*authRequest` + the 4 config booleans (`includePeerCertificate` / `includeTlsSession` / `encodeRawHeaders` / `packAsBytes`) — `DecoderFilterCallbacks` is NOT passed in as a parameter (the §6.5 mode-agnostic-closure invariant is preserved). The SOURCE of the per-stream state capture into `*authRequest` is now those 6 callbacks.

**Cross-phase reuse intent:** ext_proc (next-anticipated §9 row consuming a structured-proto outbound to an upstream service), global_ratelimit (gRPC-trailer-based rate-limiting requiring socket addresses + TLS principal for descriptor evaluation), future ext_authz extensions (non-MVP fields that may require additional cert / connection introspection) all consume the same 6 accessors. The cross-phase-reusable framework primitive lands once at 18.2 IMPL Task 4 + amends in-place if/when additional fields surface.

Phase 18.2 is the SECOND §9 family-row to extend `DecoderFilterCallbacks` (after phase-16's `DownstreamPrincipal()`).

### 6 new EncoderFilterCallbacks accessors — symmetric extension (per phase 19.1 ADR-0174)

**AMENDMENT to 18.2-anchored framing.** ADR-0174 EXTENDS the same 6 socket/TLS/listener accessors landed at 18.2 on `DecoderFilterCallbacks` to the symmetric `EncoderFilterCallbacks` surface. The phase-19 SPEC's §5.P12 in-session empirical scrape REFUTED the BRAINSTORM §3.3 CONDITIONAL hypothesis for encode-side callback symmetry — the master-tip `EncoderFilterCallbacks` carried only `ContinueEncoding` + `EncodeHeaders/Data/Trailers` (encode-side injection) + `OverwriteBody` (ADR-0131); no socket/TLS/listener accessors. Phase 19's `response_attributes` envelope (populated at the response_headers stage per the proto's `ExternalProcessor.response_attributes` doc-comment) requires encode-side access to the same socket/TLS/listener state that decode-side `request_attributes` consumes. The 6 new methods land on `EncoderFilterCallbacks` at IMPL Task 5 per ADR-0174:

1. **`DownstreamRemoteAddr() net.Addr`** — downstream client remote address (IP + port). Populates `ProcessingRequest.attributes.source.address.socket_address` at the response_headers stage.
2. **`DownstreamLocalAddr() net.Addr`** — local listener bind address (IP + port). Populates `ProcessingRequest.attributes.destination.address.socket_address`.
3. **`DownstreamTLSServerName() string`** — downstream TLS `ConnectionState.ServerName` (SNI value sent by the client). Empty for plaintext.
4. **`DownstreamTLSPeerCertDER() []byte`** — DER-encoded leaf cert of the downstream client (presented during mTLS handshake). Nil for plaintext / non-mTLS / no-client-cert.
5. **`DownstreamProtocol() string`** — protocol used on the downstream connection (`HTTP/1.1`, `HTTP/2`).
6. **`ListenerPrincipal() string`** — listener TLS cert-derived principal (URI SAN | DNS SAN | Subject DN CN per ADR-0144 priority order applied to the SERVER cert).

**Chain-side seeding discipline UNCHANGED from ADR-0165.** The 6 chain fields (`downstreamRemoteAddr` / `downstreamLocalAddr` / `downstreamTLSServerName` / `downstreamTLSPeerCertDER` / `downstreamProtocol` / `listenerPrincipal`) already exist per ADR-0165's plumbing — they are SET ONCE at chain build time by HCM dispatch (H1 `connection.go:dispatchRequest` + H2 `h2dispatch.go:WriteH2`) BEFORE either `RunDecodeHeaders` or `RunEncodeHeaders` dispatch. **NO new chain plumbing primitives** — the chain fields are shared; the encoder-side reader methods access the same chain pointer via `*encoderCB`. The 6 NEW `*encoderCB` reader methods (added at 19.1 IMPL Task 5) return the chain field verbatim — no copy, no transformation. This is a thin reader-method extension on top of existing chain plumbing, NOT a new framework primitive.

**Cross-phase reuse intent (per ADR-0174 §Decision):** future encode-side filters consuming socket/TLS/listener state — e.g., a response-validation filter that 500s on schema violations, encode-side rate-limiting based on response shape + listener-cert identity, encode-side header-injection from downstream-cert state — all consume the same 6 accessors. The cross-phase-reusable framework primitive lands once at 19.1 IMPL Task 5 + amends in-place if/when additional fields surface.

Phase 19.1 is the FIRST §9 family-row to extend `EncoderFilterCallbacks` with socket/TLS/listener accessors (phase 14 compressor's `OverwriteBody` per ADR-0131 was the only prior encode-side framework primitive). It is also the SECOND §9 family-row to anchor a cross-phase-reusable callback-surface extension after phase 18.2 ADR-0165 — the pattern (chain-field SET-once-by-HCM-dispatch + READ-by-callback) is now confirmed cross-phase-reusable on BOTH decode + encode sides.

### 7th NEW EncoderFilterCallbacks accessor — `BufferEncodedBody` (per phase 19.2 ADR-0175)

**Phase 19.2 EXTENDS the `EncoderFilterCallbacks` surface with the 7th new accessor** — `BufferEncodedBody() []byte` — the NEW encode-side body-buffering framework primitive. This is envoy-go's FIRST encode-side body-buffering primitive (the symmetric mirror of phase-13 ADR-0128 decode-side `BufferedBody`); it is distinct from + complementary to phase-14 ADR-0131 `OverwriteBody` (which is per-call replacement only, NOT buffer-and-hold). Surface:

- **`BufferEncodedBody() []byte`** — returns the accumulated buffered body bytes captured by the chain on `DataStopIterationAndBuffer` returns from prior `EncodeData` calls. Returns the live buffer slice; the filter mutates in place (subject to the standard "the filter takes ownership of the buffer on resume" discipline). Returns nil before any data has been accumulated, mirroring the decode-side `BufferedBody()` accessor's nil-on-empty convention.

**Chain-side discipline (per `internal/filter/http/chain.go` Run`EncodeData` extension at 19.2 IMPL Task 2):**

- On `DataStopIterationAndBuffer` returns from a filter's `EncodeData`, the chain accumulates the incoming chunk into the per-encoderCB `c.encodeBuf []byte` field AND does NOT forward the chunk downstream. This mirrors `RunDecodeData`'s ADR-0128 accumulation discipline exactly.
- On `ContinueEncoding()` resume, the chain releases the (possibly-mutated) `c.encodeBuf` downstream + clears the field. The filter is responsible for header-state reconciliation (Content-Length on length-change per ADR-0175 §Decision — the encode-side analog of ADR-0128 §Decision's post-body reconciliation).
- End-of-stream signal closes the buffering window — if the filter does NOT resume by end-of-stream, the chain treats the response as truncated per the existing chain-side error path (symmetric to the decode-side discipline).
- Buffer-overflow discipline mirrors `errDecodeBufferOverflow` (per planner-time D7) — when accumulated body exceeds the configured cap, the chain emits a 500 LocalReply via the existing error path. Race-tested at IMPL Task 8 (`TestEncodeBufConcurrency_*` group).

**Cross-phase reuse intent (per ADR-0175 §Decision):** any future encode-side filter requiring full-body inspection or mutation (a hypothetical encode-side content-injection filter; a future encode-side `lua` body callback; an encode-side schema-validation filter) composes against `BufferEncodedBody()` without re-deriving the buffer-and-hold discipline. The primitive lands once at 19.2 IMPL Task 2; future encode-side filters extend in-place.

**NOT-REUSED at 19.2:** phase-14 ADR-0131 `OverwriteBody(b []byte)` per-call replacement primitive — distinct semantics; both stay on `EncoderFilterCallbacks` post-19.2 as complementary primitives.

Phase 19.2 is the SECOND §9 family-row to add a NEW framework primitive on the encode side (phase 19.1 ADR-0174 added the 6 socket/TLS/listener accessors via existing chain plumbing; phase 19.2 ADR-0175 introduces NEW chain-side plumbing — the per-encoderCB `encodeBuf` field + the `RunEncodeData` accumulation extension).

---

## Matcher engine framework primitive (per phase 16 ADR-0142)

*Introduced by phase 16. Justified by ADR-0142.*

Phase 16 introduces a new top-level `internal/matcher/` package providing a generic `xds.type.matcher.v3.Matcher` match-tree evaluator. The package exports:

- `matcher.New(tree *matchv3.Matcher, supportedActionTypes []string) (*Matcher, error)` — parses the match tree + validates terminal action TypeURLs against the caller's allow-list at config-load time; PARSE-REJECT for unknown TypeURLs with envoy-go-only error.
- `matcher.Evaluate(ctx MatchContext) (*anypb.Any, error)` — walks the match tree at request time + returns the matched terminal action TypedExtensionConfig (wrapped as `Any`) OR `(nil, nil)` for no-match.
- `matcher.MatchContext` interface — the caller (a filter) implements this on its per-stream `*filter` to expose request accessors (headers, IP, principal, etc.).

Phase 16's rbac filter consumes the primitive with `supportedActionTypes = ["type.googleapis.com/envoy.config.rbac.v3.Action"]` (the canonical RBAC terminal; per phase 16 SPEC §11.P3); future filters extend the allow-list as new terminal types land.

Cross-phase reuse intent: ext_authz, jwt_authn, oauth2 all use the same `xds.type.matcher.v3.Matcher` primitive for parts of their config surface. Each future filter extends `supportedActionTypes` + widens `MatchContext` additively.

---

## JWKS framework primitive (per phase 17 ADR-0150)

*Introduced by phase 17. Justified by ADR-0150.*

Phase 17 introduces a new top-level `internal/jwks/` package providing an HTTP-outbound JWKS fetcher with a thread-safe cache + background refresh. The package exports:

- `jwks.New(uri string, cacheDuration time.Duration, asyncFetch *AsyncFetch, retryPolicy *RetryPolicy) (*Fetcher, error)` — constructs a `Fetcher`; performs a BLOCKING initial fetch when `fast_listener` is unset/false (default — initial-fetch failure returns an error and fails listener-load per ADR-0150 §Decision (iii)), or a non-blocking initial fetch when `fast_listener: true`.
- `(*Fetcher) Get(ctx) (*JWKSet, error)` — returns the cached `*JWKSet`, or `ErrJwksNotReady` while a non-blocking initial fetch is still in flight.
- `(*Fetcher) Close() error` — stops the background refresh goroutine; idempotent.
- `(*JWKSet) Lookup(kid, alg string) (crypto.PublicKey, error)` — Envoy `pickKeyAlgWithKid` lookup (match on `kid` first; fall back to first key with matching `alg`).
- `ParseJWKSet(raw []byte) (*JWKSet, error)` — RFC 7517 §5 JWK Set JSON parsing; RSA + EC (P-256/384/521) key types.

**Refresh schedule:** on a successful fetch, the next refresh is scheduled at `cacheDuration - 5s` via `time.AfterFunc` (clamped to 0 if `cacheDuration < 5s`); default cache duration is 10 minutes. **Failed-refetch schedule:** a fixed 1s interval (configurable via `AsyncFetch.FailedRefetchDuration`); NO exponential backoff (phase 17 SPEC §11.P4 REFUTED the exponential-backoff hypothesis). Inner-HTTP-request retries are honored via the `RetryPolicy` config (NumRetries + BaseInterval + MaxInterval).

**Observability:** each RemoteJwks provider whose initial blocking fetch succeeds credits the filter's `jwks_fetch_success` counter once at filter-load time; a failed request-time `Get()` increments `jwks_fetch_failed` (per ADR-0154 §Decision (vii)). Initial-fetch failure under the default (blocking) mode fails listener-load loudly rather than activating a degraded listener.

**Cross-phase reuse intent:** future filters consuming outbound-HTTP-from-filter primitives — `ext_authz` HTTP-mode, `oauth2` token-endpoint flows — compose against the `Fetcher` type's `http.Client` + retry-policy structure.

**REFACTORED-AT-PHASE-20 (per ADR-0150 §Decision AMENDMENT — landed at phase-20 IMPL Task 2 paired with the ADR-0177 NEW `internal/httpclient/` framework primitive introduction).** The `internal/jwks/Fetcher` constructor no longer owns its own `http.Client`; it now takes a `*httpclient.Client` constructor argument per the ADR-0177 framework-primitive consumption pattern. Concrete signature shift: `jwks.New(httpClient *httpclient.Client, uri string, cacheDuration time.Duration, asyncFetch *AsyncFetch, retryPolicy *RetryPolicy) (*Fetcher, error)` — the `*httpclient.Client` argument is wired at filter-construction time via the boot-shared singleton (per `cmd/envoy-go/main.go`'s shared client + the `FactoryCtx` threading per ADR-0177 §Decision). Inner-HTTP-request retries continue honored via the existing `RetryPolicy` config (no behavioral change at the wire — the refactor is pure thin-wrapper-substitution; `internal/httpclient/Client.Do` is the new dispatch site that was previously inline in the Fetcher). **3 consumers at phase-20 phase-done**: jwks Fetcher (this refactor) + extauthz httpAuthClient (post-ADR-0159 in-place AMENDMENT per the `## HTTP outbound auth-check framework note` CLOSURE-AT-PHASE-20 paragraph below) + oauth2 token_endpoint POST (NEW at phase-20). The cross-phase-consumer disposition CLOSES the implicit forward-pointer to a future httpclient primitive that ADR-0150 §Consequences carried since phase 17. Per ADR-0044 in-place edit discipline — NOT a new ADR; ADR-0150 evolves in-place with the AMENDMENT paragraph dated 2026-05-17 and cross-referenced to phase 20 + ADR-0177. Cross-package regression matrix per phase-20 SPEC §12 item C8: fixture-0019 (jwt_authn) GREEN post-refactor (RATIFIED at phase-20 IMPL Task 2b regression check); zero behavioral delta.

---

## HTTP outbound framework primitive (per phase 20 ADR-0177)

*Introduced by phase 20. Justified by ADR-0177. Related: ADR-0150 §Decision AMENDMENT (jwks Fetcher refactor at phase 20 IMPL Task 2b); ADR-0159 §Decision AMENDMENT (extauthz httpAuthClient refactor at phase 20 IMPL Task 2c) + ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph.*

Phase 20 introduces a new top-level `internal/httpclient/` package providing a generic HTTP-outbound primitive for filters that dispatch synchronous HTTP requests to upstream services. The package exports:

- **`httpclient.New(opts Options) *Client`** — constructs a `*Client` from the `Options` envelope; zero-value `Options` is a no-op (zero `Timeout` = no deadline; zero `RetryPolicy` = no retries; nil `TLSConfig`). The `*Client` is the public surface filters consume.
- **`(*Client) Do(req *http.Request) (*http.Response, error)`** — executes the request synchronously over the underlying `*http.Client`; honors `ctx` cancellation propagated through `req.Context()`; applies retries per `Options.RetryPolicy`.
- **`Options`** struct — carries per-Client configuration: `Timeout time.Duration` (applied as `http.Client.Timeout`); `RetryPolicy RetryPolicy` (zero = no retries — matches Envoy v1.37.2 wire default per phase-20 SPEC §20.P1 RATIFIED); `TLSConfig *tls.Config` (wired through the underlying `*http.Transport`).
- **`RetryPolicy`** struct — carries the optional retry envelope: `Attempts int` (0 = no retries); `PerAttemptDelay time.Duration`; `RetryOnStatus []int` (e.g. `[500, 502, 503, 504]` for retry-eligible HTTP statuses).

**3 consumers at introduction time** (the third-consumer trigger condition that fired the extraction per phase-20 BRAINSTORM Q2 + ADR-0159 §Future Work):

1. **jwks Fetcher** (post-ADR-0150 in-place AMENDMENT at phase-20 IMPL Task 2b) — `internal/jwks/fetcher.go` refactored to consume `*httpclient.Client` via constructor argument; REPLACES the inline `http.Client` ownership.
2. **extauthz httpAuthClient** (post-ADR-0159 in-place AMENDMENT at phase-20 IMPL Task 2c) — `internal/filter/http/extauthz/check.go::httpAuthClient` refactored to consume `*httpclient.Client` via constructor argument; REPLACES the inline `http.Client` ownership; **CLOSES the ADR-0159 §Future Work forward-pointer** ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction" — the third-consumer trigger fires exactly as ADR-0159 anticipated; FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer).
3. **oauth2 token_endpoint POST** (NEW at phase 20) — `internal/filter/http/oauth2/oauth_client.go::postTokenEndpoint` consumes the new primitive via `cc.httpClient.Do(req)` at the async-resume continuation site (parked by `StopIteration` per phase-09 async-resume primitive REUSE).

**Cross-phase reuse intent (per ADR-0177 §Future Work — NEW forward-pointer):** future filters consuming outbound-HTTP from a filter compose against `*httpclient.Client` directly. The next-cross-consumer event is anticipated at future ext_authz mTLS (`TLSConfig` field activation), future jwt_authn alternative-issuer fetch (a second JWKS consumer beyond the current single-issuer-per-provider shape), future ratelimit gRPC TLS (when global_ratelimit lands with operator-config-driven TLS-to-rate-limit-cluster), each at its own future trigger condition.

**Singleton boot lifecycle (per ADR-0177 §Decision + the `cmd/envoy-go/main.go` shared-client pattern landed at IMPL Task 2b + 2c):** the `*httpclient.Client` is constructed ONCE at process boot from a base `Options` envelope and threaded through to filter constructors via the `FactoryCtx` carrier — the same `*Client` is shared across all jwks Fetchers + all extauthz httpAuthClient instances + all oauth2 filter instances. The shared client is goroutine-safe (composes around `*http.Client` which is goroutine-safe per the stdlib contract); no per-filter `http.Transport` proliferation. Per-call timeouts ride through `req.Context()` (the per-Filter Options `Timeout` field stays available for per-Client overrides when a filter needs a tighter envelope than the boot default).

---

## Filesystem-SDS framework primitive (per phase 20 ADR-0178)

*Introduced by phase 20. Justified by ADR-0178. Related: NEW go.mod dependency on `github.com/fsnotify/fsnotify`.*

Phase 20 introduces a new top-level `internal/sdsfile/` package providing a filesystem-watching primitive for filters that consume operator-managed secret material (HMAC secrets, client secrets, AES-key derivation sources, future TLS-trust-store reload, future mTLS material). The package exports:

- **`sdsfile.New(path string) (*Watcher, error)`** — constructs a `Watcher` from a filesystem path pointing at the outer Secret-proto JSON/YAML file (the canonical `envoy.extensions.transport_sockets.tls.v3.Secret` proto containing `generic_secret.inline_string` ONLY; the inner `secret_file` indirect arm PARSE-REJECTs per ADR-0178 §Decision + phase-20 SPEC §8 item 14).
- **`(*Watcher) Start() error`** — begins watching; runs the fsnotify event loop in a goroutine.
- **`(*Watcher) Current() []byte`** — returns the current in-memory copy of the `inline_string` bytes (atomic load via `atomic.Pointer[[]byte]`).
- **`(*Watcher) Close() error`** — stops the goroutine and releases the fsnotify watcher; idempotent.

**`core.ConfigSource` oneof dispositions per ADR-0178 + phase-20 SPEC §3.2 + §20.P6 RATIFIED:** honors `PathConfigSource` (oneof arm field 8 wrapper; non-deprecated; wraps `{path, watched_directory}`). The deprecated `core.ConfigSource.path` field 1 PARSE-REJECTs at parse-time. The `ApiConfigSource` + `Ads` oneof arms PARSE-REJECT permanently (out-of-envelope; no in-tree xDS substrate at phase 20). The `generic_secret.secret_file` inner-indirect arm PARSE-REJECTs per phase-20 SPEC §8 item 14 (the framework watches the outer Secret-proto JSON/YAML file via fsnotify — the inner double-indirect loading is not modeled at MVP).

**~100ms debounce window (per ADR-0178 §Decision + phase-20 SPEC §12 item B7 RATIFIED at IMPL Task 3):** captures both atomic-rename-via-mv (common for operator-managed secret rotation via `kubectl create secret --dry-run | kubectl replace -f -` patterns) AND in-place-write-via-truncate-and-rewrite (less common but possible under direct-edit scripts) event sequences without false-positive reloads. The debounce coalesces a burst of fsnotify events into a single atomic-swap reload at the trailing edge.

**Atomic-swap discipline (per ADR-0178 §Decision):** the in-memory bytes live behind an `atomic.Pointer[[]byte]` — concurrent readers see consistent bytes; reload swaps the pointer atomically after a successful read of the new file content. Race-tested at IMPL Task 3 unit-tests (`TestWatcher_DebounceRace_*` group per phase-20 SPEC §14.1 item 7) with zero data-race violations under the race detector.

**MVP consumer: oauth2** — both `OAuth2Credentials.token_secret` + `OAuth2Credentials.hmac_secret` consume the primitive at phase-20 introduction. The `hmac_secret` bytes feed both the 5-input HMAC composition per phase-20 AMEND-2 + ADR-0179 AND the AES-256-CBC KDF (`SHA-256(hmacSecret)[:32]`) per phase-20 AMEND-1 + ADR-0182 — a single `*sdsfile.Watcher` instance serves multiple consumer code paths within the oauth2 filter.

**Cross-phase reuse forward-pointer (per ADR-0178 §Future Work — NEW):** future filesystem-SDS consumers anticipated: future jwt_authn TLS-trust-store reload (when a future enhancement lands operator-rotation of trust-store bundles); future ext_authz mTLS (when the `TLSConfig` axis activates per ADR-0177 future work); future ratelimit gRPC TLS (when global_ratelimit lands with operator-config-driven TLS material). Each future consumer composes against `*sdsfile.Watcher` directly; the cross-phase reuse intent is recorded as a NEW forward-pointer at the phase-20 SPEC commit.

---

## JWT verifier framework primitive (per phase 17 ADR-0151)

*Introduced by phase 17. Justified by ADR-0151.*

Phase 17 introduces a new top-level `internal/jwt/` package providing a pure-Go-stdlib JWS/JWT parser + signature verifier + claim validator. The package exports:

- `jwt.Parse(raw string) (*Token, error)` — parses the 3-part `header.payload.signature` structure (base64url-decode + JSON-decode); rejects malformed input with `ErrJwtBadFormat` + the per-segment parse-error sentinels.
- `(*Token) VerifySignature(key crypto.PublicKey, alg string) error` — RS+ES algorithm allow-list dispatch via `crypto/rsa.VerifyPKCS1v15` (RS256/384/512) + `crypto/ecdsa.Verify` (ES256/384/512); unsupported algorithms (HS family + EdDSA + `none` + PS family) return `ErrJwtHeaderNotImplementedAlg`.
- `(*Token) ValidateClaims(opts ValidateOptions) error` — validates `exp` → `nbf` → `iss` → `aud` in that order, with clock-skew tolerance (default 60s) applied to `exp` + `nbf`.
- `(*Token) PayloadClaim(path string) (interface{}, error)` — dot-notation extractor for nested claims; array-valued claims rejected with `ErrArrayClaim` per §11.P10.

**Algorithm allow-list:** RS256/384/512 + ES256/384/512 (6 algorithms). HS family + EdDSA + `none` + PS family are DEFERRED — `VerifySignature` returns `ErrJwtHeaderNotImplementedAlg` for any of them.

**Claim validation order:** `exp` → `nbf` → `iss` → `aud`. `ValidateOptions` carries `Issuer` + `Audiences` + `ClockSkew` + `Now`, plus 3 silent-ignored v1.37.x fields (`RequireExpiration` + `MaxLifetime` + `Subjects` — accepted at the API but never enforced, per §1.1 amendment 3).

**Error sentinels:** ~20 canonical error sentinels mirror jwt_verify_lib's `getStatusString()` status table — the body strings (`"Jwt is missing"`, `"Jwt is expired"`, `"Jwt verification fails"`, `"Audiences in Jwt are not allowed"`, …) are byte-exact with reference Envoy v1.37.2's deny-path bodies.

**Cross-phase reuse intent:** future filters consuming JWT semantics — `jwt_claim_router` routing on claim values, `oauth2` token validation — compose against `Parse` + `Token.VerifySignature` + `Token.ValidateClaims` + `Token.PayloadClaim` directly.

---

## gRPC client framework primitive (per phase 18.2 ADR-0158)

*Introduced by phase 18.2. Justified by ADR-0158. Related: ADR-0166 (cluster-manager plaintext h2c upstream relaxation, same-phase same-fixture amendment).*

Phase 18.2 introduces a new top-level `internal/grpcclient/` package — envoy-go's FIRST gRPC infrastructure of any kind — providing a generic gRPC outbound primitive for filters that dispatch structured-proto Check / Process / Limit calls to an upstream gRPC service. The package exports:

- **`grpcclient.New(cm *cluster.Manager) *Dialer`** — constructs a `Dialer` bound to the cluster manager. The `Dialer` is the public surface filters consume to obtain `*grpc.ClientConn` instances keyed by upstream cluster name.
- **`(*Dialer) DialContext(ctx context.Context, clusterName string) (*grpc.ClientConn, error)`** — looks up the upstream `*cluster.Cluster` by name + returns a (cached, per-cluster-singleton) `*grpc.ClientConn` constructed via `grpc.NewClient("passthrough:///"+clusterName, grpc.WithContextDialer((*cluster.Cluster).Dial), grpc.WithTransportCredentials(insecure.NewCredentials()))`. PARSE-REJECTs if the cluster does not exist OR `cluster.UseH2() == false` (gRPC requires HTTP/2).
- **`grpcclient.NewAuthClient(dialer *Dialer, clusterName string, timeout time.Duration) (*AuthClient, error)`** — constructs an ext_authz-typed wrapper that owns a per-cluster `*grpc.ClientConn` + the canonical `envoy.service.auth.v3.Authorization/Check` stub from go-control-plane v1.32.4 (no codegen). The `timeout` is the per-Check deadline applied via `context.WithTimeout`.
- **`(*AuthClient) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error)`** — synchronous Check call; honors `ctx.Done()` for cancel propagation; returns the parsed `*CheckResponse` or a gRPC-transport error.

**Connection lifecycle (per ADR-0158 §Decision):**

- **One `*grpc.ClientConn` per `(cluster_name, compiledConfig)` pair**, created at config-load time in `buildGRPCCheckFn` (NOT lazily at first request). gRPC's transport layer manages its own reconnect / backoff state; the connection is a long-lived shared resource across all per-stream `Check()` calls on the same `compiledConfig`.
- **TLS terminates at the cluster-manager layer, NOT at gRPC's TLS layer** (per the 18.2 SPEC §11.P13 in-session SPEC RATIFICATION). The `grpc.WithContextDialer((*cluster.Cluster).Dial)` integration hands gRPC a pre-dialed `net.Conn` from the cluster manager — for TLS upstream clusters that `net.Conn` is already a `*tls.Conn` (the cluster manager does the TLS handshake); for plaintext upstream clusters (per ADR-0166's relaxation) the `net.Conn` is a plaintext h2c socket. `grpc.WithTransportCredentials(insecure.NewCredentials())` informs gRPC NOT to attempt its own TLS handshake — the `net.Conn` is already in the correct mode for the upstream cluster's configuration. This is the standard gRPC-go integration pattern for the "host-managed transport" use case.
- **Closed-on-process-exit for MVP** — no explicit `Close()` call (per 18.2 SPEC §8 item 9). Matches Envoy's gRPC-client lifecycle convention. Future xDS-CDS hot-replacement lands a `Close()` discipline.

**ADR-0166 cluster-manager amendment (same phase, fixup-time, unanticipated):** the prior `cluster.Manager.extractH2Mode` / `dial_h2.go` TLS-required gate is RELAXED to permit plaintext h2c upstream clusters. Rationale: fixture 0021's in-process test-helper auth server is the FIRST envoy-go test consumer needing a plaintext h2c upstream dial path; the previous TLS-required gate (inherited from phase-05.2's upstream-H2 implementation) was incidental to TLS-fronted-only origination assumptions, not a transport-correctness constraint. ADR-0166 documents the gate relaxation + the plaintext h2c dial path + the cross-phase reuse intent for future gRPC-bridge / gRPC-Web / gRPC-JSON transcoding / H2-upstream-only filters. The relaxation is a cluster-manager amendment, NOT a TLS-layer lift — TLS upstream clusters still wrap a TLS `net.Conn` via the existing TLS handshake path; the gate relaxation simply lets plaintext upstream clusters with `http2_protocol_options{}` set route through the H2-dial path without the prior TLS-required precondition.

**Cross-phase reuse intent (per ADR-0158 §Decision):**

- **`envoy.filters.http.ext_proc`** (anticipated next-or-near §9 row): the `Process()` streaming RPC against an upstream `envoy.service.ext_proc.v3.ExternalProcessor` service. Composes against `*grpcclient.Dialer` + adds an `ext_proc`-typed wrapper analogous to `AuthClient`. The bidirectional streaming surface is more complex than ext_authz's unary Check; the dialing + transport-credential plumbing is identical.
- **`envoy.filters.http.global_ratelimit`** (anticipated §9 row): the `ShouldRateLimit()` unary RPC against an upstream `envoy.service.ratelimit.v3.RateLimitService` service. Composes against `*grpcclient.Dialer` + adds a `ratelimit`-typed wrapper. Surface is structurally identical to `AuthClient` (unary RPC + per-call deadline + transport-error → `error` disposition).
- **Future gRPC-family filters** (gRPC bridge, gRPC-Web, gRPC-JSON transcoding) compose against `*grpcclient.Dialer` for their own outbound gRPC calls.

**§11.P13 in-session SPEC RATIFICATION note (per 18.2 SPEC §11.P13):** the SPEC's prior RATIFIED-PENDING-IMPL-TIME pin for the gRPC dial / TLS-to-auth-cluster plumbing was CLOSED RATIFIED at the 18.2 SPEC commit's in-session scrape (TLS-fronted downstream listener + TLS-fronted gRPC auth cluster + Go gRPC `Authorization/Check` server fronted with self-signed TLS; verbatim CheckRequest captured at SPEC §11). 18.2 IMPL therefore had zero RATIFIED-PENDING pins — the §11.P13 closure REMOVED the most-likely ADR-0044 escape-valve trigger surface (cluster-manager coupling for `EnvoyGrpc` cluster-name resolution + TLS-to-auth-cluster plumbing was confirmed clean with no new TLS-layer lift needed). The ADR-0044 escape-valve nonetheless fired at PLAN time (D12 → ADR-0165 callback-surface extension) + at IMPL-fixup time (ADR-0166 plaintext h2c upstream relaxation) — two unanticipated ADRs landed at 18.2 anyway, illustrating that the SPEC's RATIFIED-PENDING pin closure protects against ONE surface but cannot anticipate orthogonal ADR-0044 trigger surfaces (the callback gap was a CONFIG-SCRAPE finding; the plaintext h2c gap was a FIXTURE-TOPOLOGY finding).

### Phase 19.1 EXTENSION — `*ProcessorClient` bidi-stream wrapper (per ADR-0169)

Phase 19.1 ADDS a NEW typed wrapper to the same `internal/grpcclient/` package — `*ProcessorClient` — alongside the existing unary `*AuthClient`. The package's public surface extends with:

- **`grpcclient.NewProcessorClient(dialer *Dialer, clusterName string, timeout time.Duration) (*ProcessorClient, error)`** — constructs an ext_proc-typed wrapper that owns a per-cluster `*grpc.ClientConn` + the canonical `envoy.service.ext_proc.v3.ExternalProcessor/Process` stub from go-control-plane v1.32.4 (no codegen). The `timeout` is the per-stage message timeout applied via `context.WithTimeout` per ADR-0171 §Decision (vi) cancel-and-rebuild discipline.
- **`(*ProcessorClient) Process(ctx context.Context) (ProcessStream, error)`** — opens a fresh bidi-stream for each HTTP transaction; the returned `ProcessStream` exposes `Send(*ProcessingRequest) error` + `Recv() (*ProcessingResponse, error)` + `CloseSend() error` for per-stage Send/Recv pair sequencing. Per-stream `Process()` calls are cheap (HTTP/2 stream multiplexing within the shared `*ClientConn`); ext_proc opens one bidi-stream per HTTP transaction, NOT one per stage.
- **`(*ProcessorClient) Close() error`** — closes the underlying `*grpc.ClientConn`; idempotent.

**Bidi-stream lifecycle (per ADR-0169 §Decision):** the bidi surface is the FIRST envoy-go consumer of gRPC's bidirectional streaming RPC pattern. Per-stream `Process()` opens a fresh bidi-stream; the per-stage `Send/Recv` pair runs sequentially (single-in-flight-message correlation per ADR-0171 §Decision (iii)); `CloseSend()` fires after the final stage's response completes OR on `ImmediateResponse` arrival OR on `OnDestroy` cancellation (the stream context cancel aborts any in-flight Recv promptly). The dial layer is UNCHANGED from ADR-0158 — same `*Dialer` shape, same `grpc.WithContextDialer((*cluster.Cluster).Dial)` integration, same `grpc.WithTransportCredentials(insecure.NewCredentials())` discipline. **NO `Dialer` API changes** — the bidi-stream wrapper is a peer-package typed wrapper, not a framework extension.

**Cross-phase reuse (per ADR-0169 + ADR-0158 cross-references):** any future bidi-stream gRPC filter (a hypothetical streaming OAuth2 token-refresh, a streaming metrics extension, a future generalized streaming-RPC family) composes against the same `*Dialer` + adds its own typed bidi-stream wrapper. The discipline mirrors the unary `*AuthClient` cross-phase reuse intent — one typed wrapper per gRPC service type; shared dial layer.

### JSON codec note (per phase 19.1 ADR-0170 — lighter-touch reference)

Phase 19.1 introduces a **filter-local protojson codec** at `internal/filter/http/extproc/json.go` for the `http_service` mode transport. Surface:

- **`marshalProcessingRequest(req *ProcessingRequest) ([]byte, error)`** — serializes `*ProcessingRequest` → JSON bytes via `protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: false, UseEnumNumbers: false}` per the §19.P8 closure (the three options match reference Envoy v1.37.2's wire shape on the codec-options axis; three envelope-content divergences DEFERRED to 19.2 per ADR-0170 §Consequences — protojson whitespace non-determinism per Go protobuf PR #1564; empty-message emission for `metadata_context:{}` + `protocol_config:{}`; writer-side `value` vs `raw_value` choice at attributes.go).
- **`unmarshalProcessingResponse(data []byte) (*ProcessingResponse, error)`** — deserializes JSON bytes → `*ProcessingResponse` via `protojson.UnmarshalOptions{DiscardUnknown: true}` per the §19.P8 Phase 3b investigation (reference Envoy's HTTP-mode processor injects forward-compat unknown fields that envoy-go must tolerate).

The codec is **filter-local** per the phase-18.1 ADR-0159 (b)-disposition rationale — NOT a SHARED cross-phase primitive. The natural trigger to generalize into `internal/jsoncodec/` is the SECOND consumer (anticipated: a future filter consuming a `protojson`-encoded outbound HTTP payload — e.g., a webhook filter). When that lands, the generalization should be reconsidered. ADR-0170 records the disposition + the second-consumer forward-pointer.

NOTE: this is a lighter-touch reference (~20 LoC of surface documentation) under the `## gRPC client framework primitive` umbrella, NOT a new top-level framework umbrella. The JSON codec is a transport-layer detail orthogonal to the gRPC client primitive; both compose into the dual-mode `processorClient` interface at the ext_proc package layer per ADR-0167 + ADR-0168.

### Phase 19.2 EXTENSION — body-stage activation + chain-side body-buffering note (per ADR-0168 §Decision AMENDMENT + ADR-0171 §Decision AMENDMENT + ADR-0172 §Decision AMENDMENT + ADR-0175)

Phase 19.2 ACTIVATES the body-mode arms of the ext_proc filter against the existing 19.1 package surface — NO new files in `internal/filter/http/extproc/`; the 4-stage state machine extends the existing `processor.go` 2-stage table; the body-stage `applyProcessingResponse` arms extend the existing `check.go` 7-step dispatcher; the body-stage attribute envelope builders extend `attributes.go` with `buildRequestBodyProcessingRequest` + `buildResponseBodyProcessingRequest` + `buildBodyAttributeEnvelope` per IMPL Task 5; the `compiledConfig` struct stays field-final per ADR-0168 §Decision (xi). The bidi-stream `*ProcessorClient` REUSES unchanged (one bidi-stream per HTTP transaction; body-stage outbound rides the same `Send/Recv` pair as the header-stage outbound; the `ProcessStream` 3-method interface is UNCHANGED).

**Chain-side body-buffering primitive note (per ADR-0175 — cross-package extension at `internal/filter/http/`):** the NEW `EncoderFilterCallbacks.BufferEncodedBody() []byte` accessor + the `RunEncodeData` accumulation extension land at `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` per IMPL Task 2. The primitive is **cross-phase-reusable** beyond the ext_proc filter (future encode-side body-transformation filters consume it without re-deriving the buffer-and-hold discipline; see `## HTTPFilterCallbacks → ### 7th NEW EncoderFilterCallbacks accessor` for the per-method documentation + the chain-side discipline). At 19.2 the sole CONSUMER is `internal/filter/http/extproc/`'s `response_body` stage; the cross-phase reuse intent is recorded in ADR-0175 §Decision for the SECOND-consumer trigger.

**Per-message timer behavioral enforcement (per ADR-0171 §Decision AMENDMENT at IMPL Task 4):** the 19.1 STRUCTURAL-ONLY carry-forward CLOSES at 19.2 — a single rolling per-direction timer via `context.WithTimeout` cancel-and-rebuild (planner-time D4) bounds each in-flight `Recv` against the active per-message timeout (default 200ms; overridable per stage within `[1ms, max_message_timeout]` when `max_message_timeout ≥ 1ms`). The timer fires `streamCancel()` on expiration → in-flight Recv returns `ctx.Err()` → mapTransportError applies `failure_mode_allow` posture per the 19.1 error-posture discipline.

**No new ADR consumed at 19.2 IMPL (D10 hypothesis HELD):** the planner-time D10 hypothesis predicted NO impl-time-unanticipated ADR fires at 19.2 IMPL; the SPEC-time anticipated surfaces (buffered-body-release-vs-stream-reset interaction; mutation_rules application to body bytes) settled within the existing ADR envelope. ADR-0177 stays unconsumed (reserved for future-phase surfaces).

---

## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)

*Introduced by phase 18.1. Justified by ADR-0159.*

Phase 18.1 introduces a per-request HTTP-outbound auth-check in `internal/filter/http/extauthz/check.go`. This is NOT a new shared cross-phase primitive (unlike phase-17's `internal/jwks/` + `internal/jwt/`); it is ext_authz-local per ADR-0159 disposition (b). The decision rationale: two consumers (`jwt_authn`'s JWKS fetcher + `ext_authz`'s auth-check) whose lifecycles barely overlap do NOT justify a shared `internal/httpclient/` package at this time.

**Structural composition against phase-17 `internal/jwks/Fetcher`:** `check.go` reuses the same `net/http.Client` shape — `Timeout` + `Transport` (default `http.DefaultTransport`) — that `internal/jwks/Fetcher` established as the envoy-go outbound-HTTP idiom. There is no shared type or import; the parallel is structural (same field names, same timeout wiring, same `ctx`-cancel propagation).

**Per-request lifecycle:** each `DecodeHeaders` call fires a goroutine that POSTs to the configured `http_service.server_uri`; the dispatch goroutine parks via `StopIteration` + a per-stream resume channel (mirrors the phase-09 fault async-resume primitive). `OnDestroy` cancels the in-flight call's `context.Context` — the FIRST §9 row with a per-request cancellable outbound call. A canceled context returns a `context.Canceled` error; the filter maps it to the `error` stat counter and applies the configured `failure_mode`.

**`internal/httpclient/` generalization trigger (forward-pointer):** the natural trigger for extracting a shared `internal/httpclient/` package is the THIRD outbound-HTTP consumer — anticipated `oauth2` token-endpoint flows (synchronous-per-request POST; same shape as ext_authz's auth-check). When the `oauth2` phase brainstorms, the generalization should be reconsidered with three consumers in view. See `## JWKS framework primitive (per phase 17 ADR-0150)` above for the outbound-HTTP structural precedent, and `## Forward-pointer notes → Phase 18.1 forward-pointer notes` below for the forward-pointer entry.

**CLOSURE-AT-PHASE-20 (per ADR-0159 §Decision AMENDMENT + §Future Work CLOSED-AT-PHASE-20 paragraph — landed at phase-20 IMPL Task 2c paired with the ADR-0177 NEW `internal/httpclient/` framework primitive introduction).** The third-consumer trigger condition that ADR-0159 §Future Work anticipated FIRED exactly as scoped at phase 20: the oauth2 token_endpoint POST is the THIRD outbound-HTTP-from-filter consumer alongside the phase-17 jwks Fetcher + the phase-18.1 extauthz httpAuthClient. The shared `internal/httpclient/` package is EXTRACTED per ADR-0177; `extauthz/check.go::httpAuthClient` is REFACTORED in-place to consume `*httpclient.Client` via constructor argument (the inline `http.Client` ownership is REMOVED; ~50-80 LoC delta in `internal/filter/http/extauthz/`); the cross-package regression matrix per phase-20 SPEC §12 item C8 is GREEN (fixture-0020 ext_authz HTTP-mode + fixture-0021 ext_authz gRPC-mode both unchanged post-refactor; RATIFIED at phase-20 IMPL Task 2c regression check). **This is the FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer** — the load-bearing demonstration is recorded as a structurally important milestone per phase-20 SPEC §9 item 1 + BRAINSTORM §11 Lesson (d). Per ADR-0044 in-place edit discipline — NOT a new ADR; ADR-0159 evolves in-place with the §Decision AMENDMENT paragraph + the §Future Work CLOSED-AT-PHASE-20 closure paragraph clearly dated 2026-05-17 and cross-referenced to phase 20 + ADR-0177. See `## HTTP outbound framework primitive (per phase 20 ADR-0177)` above for the framework primitive umbrella + the 3-consumer roster.

---

## Forward-pointer notes

### Phase 11 forward-pointer notes

**Deferred field families** (silent-ignored per ADR-0040; see `### envoy.filters.http.local_ratelimit ### Silent-ignored fields` above + phase 11 SPEC §2.1 for the full 14-field list):

- Descriptor-action subsystem (4 fields) → couples to `global_ratelimit` future phase under `BOOTSTRAP_PROMPT.md` §9 HTTP filters family.
- Runtime + shadow-mode subsystem (3 fields, including `filter_enabled` and `filter_enforced` `RuntimeFractionalPercent` fields) → couples to Runtime + hot restart family. **Divergence-window:** envoy-go silent-ignores these fields; reference Envoy defaults both to 0% (off). Differential fixture configs MUST set both to 100% explicitly; users running envoy-go with these fields set to non-100% values will diverge from Envoy (envoy-go behaves as always-100%, Envoy honors the percentage).
- xDS cluster-state (1 field: `local_cluster_rate_limit`) → couples to xDS / dynamic config family.
- Response-side header injection (1 field: `response_headers_to_add`) → standalone follow-on.
- Per-connection lifecycle (1 field: `local_rate_limit_per_downstream_connection`) → standalone follow-on.
- Multi-stage limiting (1 field: `stage`) → couples to descriptor-action subsystem.
- X-RateLimit headers + vh policy (2 fields: `enable_x_ratelimit_headers`, `vh_rate_limits`) → standalone follow-on.
- gRPC trailer mapping (1 field: `rate_limited_as_resource_exhausted`) → couples to gRPC family.

**Tag-extraction collision quirk:** when `local_ratelimit.stat_prefix` matches an Envoy-internal tag-extractor name (`listener`, `http`, `cluster`, etc.), Envoy v1.37.2 mangles the Prometheus metric name. envoy-go's tag-extractor registration replicates the standard non-collision case; collision-mangling parity is OUT of scope for phase 11 (see phase 11 SPEC §1.1 amendment + §11.5 conclusions (e)).

**`filter_enabled`/`filter_enforced` 0%-default divergence-window:** reference Envoy v1.37.2 defaults both `filter_enabled` and `filter_enforced` `RuntimeFractionalPercent` fields to 0% (off) when unset, meaning the filter is fully disabled by default unless both fields are set to 100% in the config. envoy-go silent-ignores both fields and is effectively always-100%. The differential fixture 0013 sets both to 100% explicitly on the Envoy side; any envoy-go deployment that omits these fields while pairing with reference Envoy will see divergent behavior (Envoy disabled, envoy-go enabled). This divergence-window is documented here and in phase 11 SPEC §13.1 pending a future Runtime family phase that adds `filter_enabled`/`filter_enforced` support.

### Phase 12 forward-pointer notes

**Deferred field families** (silent-ignored / parse-validated-but-runtime-ignored per ADR-0040 + ADR-0121; see `### envoy.filters.http.csrf ### Field decomposition` above + phase 12 SPEC §2.1 for the full 3-field map):

- `filter_enabled` (`RuntimeFractionalPercent`) — PGV-required at parse-time (envoy-go validates presence of the field + its inner `default_value` per phase 12 SPEC §11.11; mirrors Envoy's PGV envelope). Silent-ignored at runtime; envoy-go always evaluates as if 100%-active. **Divergence-window:** users who explicitly set `default_value < 100%` will see Envoy gate by percentage (where `default_value=0%` short-circuits the filter entirely, all 3 counters stay at 0), envoy-go always-100%. Differential fixture 0014 sets explicit 100% on both sides for byte-equivalent equivalence. Couples to Runtime + hot restart family.
- `shadow_enabled` (`RuntimeFractionalPercent`) — OPTIONAL at parse-time (Envoy permits omission). Silent-ignored at runtime; envoy-go always-never-shadow. **Stat coupling:** when `filter_enabled=0%` and `shadow_enabled=100%` in reference Envoy, the same 3-counter family increments (request_valid / request_invalid / missing_source_origin) but no 403 is emitted; envoy-go's MVP cannot reach this state since it always evaluates as 100%-enforce. Couples to Runtime + hot restart family.
- `additional_origins[].StringMatcher` non-exact variants (`prefix`, `suffix`, `contains`, `safe_regex`, `ignore_case`) — dropped at PARSE time per ADR-0101 §3 discipline. Empty-value `exact` entries also dropped. Couples to whatever future phase lands the full StringMatcher engine (TBD; not currently a §9 family heading).

**Operator footgun (per phase 12 SPEC §11.7 + §11.8):** `additional_origins[].exact` matches the source's `host[:port]` form (NOT the full URL with scheme). Writing `exact: "https://app.example.test"` will NEVER match a real `Origin:` header. Operators MUST write `exact: "app.example.test"` (host only) or `exact: "app.example.test:443"` (explicit port). envoy-go faithfully replicates Envoy's behavior; this is a footgun in the upstream spec, NOT an envoy-go-specific quirk.

**No new tag-extractor:** csrf reuses the existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (Rule SN2 from ADR-0061). UNLIKE phase 11's local_ratelimit which introduced filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118), phase 12 introduces NO new SN flattening rule and NO new tag-extractor pattern.

**Per-route stats SHARED with listener-level:** csrf is the FIRST production filter to demonstrate the "wholesale data-only override + shared stats" pattern. Phase 11's local_ratelimit is the precedent for "wholesale stateful override + independent stats" (per ADR-0117). The two patterns coexist under ADR-0073's wholesale-override discipline; future stateful per-route filters with their own stat namespaces follow phase 11; future data-only per-route filters with HCM-scoped stats follow phase 12.

### Phase 13 forward-pointer notes

**Deferred field families** (silent-ignored / parse-rejected per ADR-0040 + ADR-0076 + ADR-0126; see `### envoy.filters.http.buffer ### Listener-level field decomposition` above + phase 13 SPEC §2.1 for the full field map):

- `Buffer.max_request_bytes > 1 MiB` (envoy-go-only PARSE-time rejection per ADR-0126) — coupled to the future cap-promotion phase (compression's natural amender per ADR-0076 §Consequences (d)). Reference Envoy accepts arbitrary `UInt32Value`; envoy-go rejects values > 1048576 at parse time with envoy-go-own error wording. **Divergence-window:** operators with existing configs targeting `max_request_bytes > 1 MiB` against reference Envoy MUST adjust their config (lower the value) to load on envoy-go. Future re-activation: when the cap-promotion phase amends ADR-0076 to make `filterBufferLimitBytes` per-stream tunable via `per_connection_buffer_limit_bytes` / `per_request_buffer_limit_bytes`, ADR-0126 amends in-place to remove the parse-time ≤ 1 MiB validation; `max_request_bytes` becomes operationally equivalent to reference Envoy.
- `per_connection_buffer_limit_bytes` (Listener-scope) / `per_request_buffer_limit_bytes` (Route-scope) — silent-ignored at parse time per ADR-0076 §Decision (d). Phase 13 does NOT change this disposition. Future re-activation: same cap-promotion phase as above.

**No new tag-extractor:** Buffer reuses the existing `envoy_http_conn_manager_prefix` Prometheus tag-extractor (Rule SN2 from ADR-0061). UNLIKE phase 11's local_ratelimit which introduced filter-specific `envoy_local_http_ratelimit_prefix` (Rule SN9 per ADR-0118), phase 13 introduces NO new SN flattening rule and NO new tag-extractor pattern. Phase 13 furthermore introduces NO new stat-table entries at all (per phase 13 SPEC §11.5 + §1.1 amendment 5 — reference Envoy emits no `envoy_http_buffer_*` counter family).

**Per-route stats SHARED with listener-level (vacuously):** Phase 13 demonstrates the disabled-OR-override per-route discipline (5th canonical shape per ADR-0125). The SHARED-stats invariant is structurally vacuous for buffer (no filter-specific counters to share or split) but documented for cross-filter consistency: future stateful per-route filters with their own stat namespaces follow phase 11 (ADR-0117 INDEPENDENT stats); future data-only per-route filters with HCM-scoped stats follow phase 12 (ADR-0124 SHARED stats); future disabled-OR-override per-route filters with NO filter-specific stats follow phase 13 (ADR-0125 SHARED-vacuous stats).

**Body-counting algorithm divergence from reference Envoy:** envoy-go's filter does its own per-stream byte-counting + 413 emission via `SendLocalReply`, while reference Envoy delegates to HCM via `callbacks_->setBufferLimit + StopIteration`. The deliberate divergence is recorded in ADR-0127 v2 §Consequences with an explicit forward-pointer to the future cap-promotion phase that may revisit this. WIRE OUTCOMES are byte-equivalent on every observable axis (status, body, headers, counter increment, `Connection: close`); only the `maybeAddContentLength` semantics are observable upstream-side as a deliberate mirror of `buffer_filter.cc:91-97`.

**Framework deltas at `internal/filter/hcm/connection.go`:** Phase 13 introduces two HCM primitives (synthetic empty-terminal `RunDecodeData` on chunked-body EOF + post-body CL reconciliation propagating filter-set `Content-Length` into `req.ContentLength`) to support the `maybeAddContentLength` observable. These are the FIRST framework deltas since phase 07.1's body-buffering machinery. Documented in ADR-0128. Future filters relying on chunked → fixed-CL conversion at the upstream boundary can rely on these primitives.

**Phase-04 `Expect: 100-continue` deferral:** envoy-go's `connection.go:122` categorically rejects any request carrying `Expect:` with 417 (pre-filter-chain guard). The buffer filter's `Expect: 100-continue` + overflow path (which would emit 100-Continue before the 413 in reference Envoy) cannot fire in envoy-go MVP. Tracked for future fix in the phase-04 Expect-handling bundle; cross-referenced in ADR-0127 v2 §Decision (v).

### Phase 14 forward-pointer notes

**Deferred field families** (silent-ignored / parse-rejected per ADR-0040 + ADR-0130; see `### envoy.filters.http.compressor ### Field decomposition` above + phase 14 SPEC §2.1 for the full field map):

- `Compressor.compressor_library` non-Gzip TypeURLs (envoy-go-only PARSE-time rejection per ADR-0130) — coupled to future codec-extension phases (brotli + zstd). Reference Envoy accepts `envoy.extensions.compression.{brotli,zstd}.compressor.v3.{Brotli,Zstd}`; envoy-go rejects with envoy-go-own error wording. Future re-activation: codec phases extend ADR-0130 + add codec-library dispatch helpers.
- `Compressor.request_direction_config` (silent-ignored at parse + runtime per ADR-0130) — coupled to future request-side compression phase + the future `envoy.filters.http.decompressor` filter.
- 4 deprecated top-level mirror fields (`content_length`, `content_type`, `disable_on_etag_header`, `remove_accept_encoding_header`) — silent-ignored at parse; operators MUST use the `response_direction_config` paths.
- `Compressor.runtime_enabled` + `response_direction_config.common_config.enabled` (RuntimeFeatureFlag fields) — silent-ignored at runtime; envoy-go always-100%-active. Couples to Runtime + hot restart family.
- `Compressor.choose_first` — always-q-value-based selection; divergence-window when set true AND multi-coding AE.
- `Compressor.response_direction_config.status_header_enabled` — always-no-status-header; the `x-envoy-compression-status:` debug header is not emitted. Operator divergence-window when set true.
- `Gzip.{memory_level, window_bits, chunk_size}` — silent-ignored; Go `compress/gzip` does not expose libz-equivalent knobs.
- Per-route `overrides.compressor_library` (per-route library swap) — silent-ignored at parse + runtime; envoy-go uses listener-level library regardless of per-route override.

**Wire-shape divergence-window from reference Envoy (per ADR-0131 + phase 14 SPEC §11.9):** Envoy emits `Transfer-Encoding: chunked` + no `Content-Length` on every compressed response; envoy-go MVP emits fixed `Content-Length: <gzipped-len>` + identity transfer. Decompressed body bytes are byte-equivalent (gzip-format multi-encoding spec admits both). Compressed body bytes structurally diverge — Go `compress/gzip` (default `OS: 255`, variable `XFL`) vs. Envoy libz (`OS: 03 UNIX`, `XFL: 00`). Future re-activation: encode-side streaming framework phase (`writeH1Reply` chunked-output mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation in HCM).

**Framework deltas at `internal/filter/http/callbacks.go` + `chain.go` + `internal/filter/hcm/connection.go` + `h2dispatch.go`:** Phase 14 introduces `EncoderFilterCallbacks.OverwriteBody(b []byte)` interface method (1 LoC) + encoderCB.OverwriteBody impl (~6 LoC at chain.go) + per-stream encode-body-override field (~2 LoC) + accessor (~3 LoC) + HCM-side post-RunEncodeData harvest at H1 + H2 dispatch paths (~6-8 LoC each). Total ~20-25 LoC. Symmetric to phase-13 ADR-0128's decode-side primitives. Future filters needing encode-side body mutation (decompressor; bandwidth_limit transform mode; future codec/transform filters) can rely on this primitive.

**HCM `directResponseAction.response_headers_to_add` plumbing (per ADR-0134):** Phase 14 uncovers an unanticipated framework gap at integration time — pre-phase-14 envoy-go's `directResponseAction.body()` hardcoded `Content-Type: text/plain` ignoring the route-level `response_headers_to_add`. The fixture-0016 scenario 2 (image/png content-type-skip path) required the route-level override to land on the direct_response body before the compressor's content-type predicate sees it. Phase 14 adds `directResponseAction.extraHeaders` + `buildExtraResponseHeaders` parser at `internal/filter/hcm/{actions.go, config.go}` (~100 LoC including 7 unit tests) with `OVERWRITE_IF_EXISTS_OR_ADD`-only AppendAction support; `APPEND_IF_EXISTS_OR_ADD` / `ADD_IF_ABSENT` / `OVERWRITE_IF_EXISTS` are reserved for future support (parse-time rejection). Fixture configs MUST set the action explicitly. Future filters with route-level direct_response interactions can rely on this primitive.

**`min_content_length` late-revert anomaly:** when Content-Length is unset at EncodeHeaders + body length emerges below threshold only at EncodeData, the filter cannot revert headers. Phase 14 documents but defers; structurally rare in envoy-go's framework. Future cap-promotion or revert-headers-primitive phase may revisit. See ADR-0131 §Decision (vii).

**Stat namespace shape:** `compressor.<library_name>.<codec>.[response.]<counter>`. `<library_name>` is operator-supplied (`compressor_library.name`); empty allowed; emits with consecutive dots. `[response.]` infix appears IFF `response_direction_config` is set on the listener-level Compressor (per compressor.proto line 158-164). Phase-14 fixture 0016 uses `name: text_optimized` on both sides + always sets `response_direction_config` for byte-equivalent stat namespace.

**Per-side empirical-divergence counters (Task 14 pin):** four counters where reference Envoy v1.37.2 + envoy-go diverge by design choice on the 0016 6-scenario workload — both sides are valid implementations of the documented contract; the differential locks both per-side via `counterModePerSideExact`:

| Counter | Ref Envoy v1.37.2 | envoy-go MVP | Root cause |
|---|---|---|---|
| `header_compressor_used` | 3 | 5 | envoy-go caches AE classification BEFORE per-route rmAE strip (ADR-0129 same-`*filter`); ref reclassifies post-strip |
| `header_not_valid` | 1 | 0 | ref's post-strip reclassification on rmAE routes returns `not_valid`; envoy-go's cached state returns `no_accept_header` |
| `response_not_compressed` | 3 | 2 | ref's per-route-disabled STILL increments despite filter being wholly inactive; envoy-go's per-route-disabled wholly silent (ADR-0125) |
| `request_not_compressed` | 6 | 0 | ref increments PER REQUEST even with response-only configs; envoy-go MVP request side silent (ADR-0132 twin-series; couples to future decompressor phase) |

Future re-activation of cross-side delta-equality on these 4 counters couples to: (a) AE-classification semantics alignment (could go either direction in a future refactor; both sides are valid); (b) per-route-disabled stat-emission discipline (ADR-0125 amendment if envoy-go decides to emit the SHARED-stats counter even on disabled-route paths); (c) request-side activation (decompressor phase / `request_direction_config` activation).

### Phase 15 forward-pointer notes

**Deferred field families** (silent-ignored per ADR-0040 + ADR-0136; see `### envoy.filters.http.bandwidth_limit ### Field decomposition` above + phase 15 SPEC §2.1 for the full 3-field silent-ignore map):

- `BandwidthLimit.runtime_enabled` (RuntimeFeatureFlag) — silent-ignored at runtime; envoy-go always-100%-active regardless of `default_value` or runtime-key state. Couples to Runtime + hot restart family. Re-activation: Runtime family phase brings RTDS / Runtime-layer support.
- `BandwidthLimit.enable_response_trailers` + `response_trailer_prefix` — silent-ignored; envoy-go always-no-trailers. Couples to a future trailer-emission framework phase that lands `EncoderFilterCallbacks.EmitTrailers(map[string]string)`. Re-activation enables 4 trailers prefixed by `response_trailer_prefix`: `bandwidth-request-delay-ms`, `bandwidth-response-delay-ms`, `bandwidth-request-filter-delay-ms`, `bandwidth-response-filter-delay-ms`.

**`limit_kbps` units are KiB/s NOT kbps (per phase 15 SPEC §1.1 amendment 6 + §11.P15 + proto comment):** The proto comment at `bandwidth_limit.pb.go:95` documents "The limit supplied in KiB/s" (kibibytes-per-second). BRAINSTORM's "kilobits-per-second" framing was empirically refuted at SPEC §11.P15. The throttle math is kbps-per-tick: `chunk_size = limit_kbps × 1024 × fill_interval_seconds` (units check: KiB/s × seconds = KiB; ×1024 = bytes). Documentation + operator-facing config commentary in fixture 0017 README + envoy-go.yaml comments consistently use KiB/s terminology.

**Histogram divergence-window (per phase 15 SPEC §1.1 amendment 9 + §11.P3):** Envoy v1.37.2 emits 2 UNCONDITIONAL transfer-duration histograms per active stat_prefix (`request_transfer_duration`, `response_transfer_duration`) — fire regardless of `enable_response_trailers` setting. envoy-go MVP per phase-06.1 baseline ("counters + gauges only — histograms deferred") emits NO histograms. Differential fixture 0017's `expectations.yaml` allow-lists via the `### Twin-series filter discipline` phase-15 extension; operator dashboards querying `envoy_<prefix>_http_bandwidth_limit_<dir>_transfer_duration_*` series see Envoy emit but envoy-go absent. Future re-activation: histogram-emit-infra phase lands `*stats.Registry.Histogram` + Prometheus `histogram_*` extractor; `filterStats` extends with 2 histogram fields.

**Wire-shape divergence-window from reference Envoy (per phase 15 SPEC §1.1 amendment 6 + ADR-0137 + §11.P8):** envoy-go's Path B-async emits ONE body-blast at the end of the throttle window; Envoy emits Path A rate-paced chunks at exact `fill_interval` cadence (chunk_size = `limit_kbps × 1024 × fill_interval_seconds` bytes per tick; e.g., 51.2 bytes/tick at kbps=1 fill=50ms; 512 bytes/tick at kbps=10 fill=50ms). **Per-side** total wall-clock throttle time observably equivalent within ±70ms tolerance per phase 15 SPEC §11.P9 (probeL: 3.904s observed vs 3950ms theoretical at kbps=1 fill=50ms body=4000B). Chunk-arrival-timing axis observably DIVERGES; HTTP clients that don't depend on intra-throttle chunk timing see byte-equivalent delivery. Future re-activation: encode-side streaming framework phase (`writeH1Reply` chunked-output mode + `EncoderFilterCallbacks.EmitChunk` + chunk-by-chunk `RunEncodeData` invocation) lands the rate-paced chunk-emit primitives, upgrading Path B-async to Path A streaming.

**Cross-side wall-clock divergence > ±70ms for initial-burst-capacity bodies — Task 14 empirical refutation of SPEC §11.P9(c) (per Task-14 finding (a)):** SPEC §11.P9(c) hypothesized "total-throttle-time converges within ±70ms" CROSS-side for all body sizes. Task-14 empirically REFUTED this for bodies within initial-burst capacity: Envoy's initial-burst-discount (the first `limit_kbps × 1024 × fill_interval_seconds × <burst-multiplier>` bytes pass without observable throttle delay) shifts the wall-clock baseline relative to envoy-go's deterministic ceil-formula. The cross-side delta can exceed ±70ms (observed up to ~200ms on tiny-body scenarios). **Adopted discipline:** each side asserted independently within ±70ms of its predicted target (per-side wall-clock) instead of cross-side delta. Fixture 0017's driver implements per-side wall-clock assertion; `## Timing tolerances` records the ±70ms per-side tolerance.

**`*_enabled` + `*_enforced` counter `counterModePerSideExact` mode rationale (per Task-14 finding (b)):** Envoy's `*_enabled` bumps per-active-side-regardless-of-body (initial-burst-discount + per-request bump-on-active-side); envoy-go's `DecodeData`/`EncodeData`-driven bump increments only when bytes actually arrive at the filter. Per-side counts diverge on tiny-body and within-burst-capacity workloads. Envoy's `*_enforced` increments per `fill_interval` tick during throttle (per phase 15 SPEC §11.P3 — probeJ shows `response_enforced: 99` for a hung 5-second stream ≈ 100 ticks at 50ms); envoy-go Path B-async bumps `*_enforced += ticks` at stream-completion to maintain cumulative byte-equivalence. The differential fixture 0017 locks all four counters per-side via `counterModePerSideExact`. The four `*_incoming_total_size` + `*_allowed_total_size` counters retain cross-side delta-equality.

**Gauges not asserted (per Task-14 finding (c)):** The 6 gauges per stat_prefix (`*_pending`, `*_incoming_size`, `*_allowed_size` × {request, response}) are NOT asserted by the differential. Rationale: transient/noisy mid-stream observations — `*_pending` is Inc-on-arm + Dec-on-fire-or-cancel and observable only mid-throttle-window; `*_incoming_size` + `*_allowed_size` are per-tick bytes-in-flight transient (envoy-go MVP single-blast: set to bodyLen at timer-fire then 0 at OnDestroy; Envoy's per-tick semantic differs). Cross-side gauge equality at a single scrape instant is structurally racy and not load-bearing for the equivalence claim. Future re-activation if/when histogram-emit-infra lands plus a steady-state assertion harness.

**envoy-go HCM synchronous-dispatch discipline — non-endStream `DataContinue` fix (per Task-14 finding (d) + phase-13 buffer precedent):** envoy-go's HCM dispatches filter callbacks synchronously on the request-handling goroutine. Returning `DataStopIterationAndBuffer` from a non-endStream `DecodeData`/`EncodeData` invocation BLOCKS the dispatch loop awaiting `ContinueDecoding`/`ContinueEncoding`, but those callbacks are invoked from within the same goroutine — deadlock. Phase 15's algorithmic correction mirrors phase-13 buffer's analogous discipline: non-endStream `DecodeData`/`EncodeData` returns `DataContinue` and accumulates the chunk locally onto `f.requestBody`/`f.responseBody`; ONLY the endStream invocation returns `DataStopIterationAndBuffer` + arms the timer + invokes the framework's buffered-return path. This diverges from SPEC §6.5 verbatim algorithm wording but produces byte-equivalent wire outcomes. ADR-0137 documents. Future filters consuming the same HCM synchronous-dispatch primitive (and needing per-chunk filter visibility) couple to a future async-HCM phase that would allow `DataStopIterationAndBuffer` on non-endStream chunks.

**Operational foot-gun: listener-level missing `limit_kbps` + active `enable_mode` causes runtime hang (per phase 15 SPEC §1.1 amendment 10 + probeJ):** Envoy's proto comment at `bandwidth_limit.pb.go:99-107` permits unset `limit_kbps` at listener-level (intended for per-route-only configurations); at runtime, an unset `limit_kbps` + active `enable_mode` causes the filter to compute infinite throttle and HANG every request. envoy-go MVP MATCHES this behavior byte-equivalently (no parse-time warning; the foot-gun is consistent across both proxies). Operators MUST set `limit_kbps` on either the listener-level config OR every per-route override. Future operator-ergonomics phase MAY add an envoy-go-side parse-time WARNING log; phase-15 position is silent-match-Envoy in MVP.

**Filter-chain ordering with respect to compressor (per phase 15 BRAINSTORM §11.6):** When both `bandwidth_limit` and `compressor` are in the same chain, ordering affects throttle-input bytes:
- `bandwidth_limit BEFORE compressor` → throttle paces the uncompressed body (more bytes through the throttle).
- `bandwidth_limit AFTER compressor` → throttle paces the compressed body (fewer bytes; tighter effective throughput).
Both orderings are valid; the contract documents the trade-off without prescribing. Fixture 0017 uses bandwidth_limit standalone for byte-equivalence simplicity.

**ZERO framework deltas:** Phase 15 introduces no new framework primitive on either side. Reuses (a) phase-09 fault's `time.AfterFunc` + `cb.ContinueDecoding/Encoding`; (b) phase-13 ADR-0128's decode-side body-buffering machinery; (c) phase-14 ADR-0131's `OverwriteBody` (anticipated: NOT invoked; the framework's buffered-return path returns bytes unchanged). FIRST §9 row to consume BOTH ADR-0128 + ADR-0131 simultaneously.

**No per-route `BandwidthLimitPerRoute` proto (per phase 15 SPEC §1.1 amendment 1 + §11.P1):** Phase 15 BRAINSTORM hypothesized a `BandwidthLimitPerRoute` oneof envelope with `disabled` + override (5th canonical disabled-OR-override per ADR-0125). Empirically REFUTED at §11.P1: no wrapper proto exists in Envoy v1.37.2; per-route TPFC uses the same `BandwidthLimit` message directly. Mirrors phase-11 local_ratelimit per ADR-0117 IMPL-1, with one additional code-level constraint: per-route entries MUST set `limit_kbps` (else Envoy rejects at boot with `"limit must be set for per route filter config"`). Phase-15 introduces a NEW canonical per-route shape (bare-message-via-TPFC + code-level-required-`limit_kbps`-at-per-route) — the 6th canonical entry, documented at ADR-0125 §(xi) amendment paragraph. The 5th canonical disabled-OR-override stays bound to phase-13 buffer + phase-14 compressor.

**No new tag-extractor:** bandwidth_limit reuses the existing `internal/stats/name.go` default-branch flatten — `<stat_prefix>.http_bandwidth_limit.<counter>` renders as `envoy_<stat_prefix>_http_bandwidth_limit_<counter>{}` via dot→underscore substitution; NO labels / NO tag-extractor / NO new SN10 rule (the BRAINSTORM-hypothesized SN10 was refuted at SPEC time when the empirical scrape showed stat_prefix inlined into the base name; ADR-0061 + ADR-0118 NOT amended).

### Phase 16 forward-pointer notes

**Deferred field families** (silent-ignored or PARSE-REJECT per ADR-0040 + ADR-0141 + ADR-0143; see `### envoy.filters.http.rbac ### Field decomposition` above + phase 16 SPEC §2 for the full 12-item deferral map):

- `RBAC.audit_logging_options` (RBAC_AuditLoggingOptions) — silent-ignored at parse + runtime; `[#not-implemented-hide:]` upstream. Couples to future audit-logging family phase.
- Policy `condition` + `checked_condition` + `cel_config` (three CEL fields per phase 16 SPEC §1.1 amendment 6) — silent-ignored at runtime per Q7. Couples to a future CEL framework phase landing `internal/cel/` + `github.com/google/cel-go`. Re-activation enables fine-grained condition evaluation.
- Permission DEFERRED set: `metadata` (deprecated; PARSE-REJECT envoy-go-only divergence-window per §11.P12 — Envoy lenient-accepts with deprecation warning); `matcher` (TypedExtensionConfig; PARSE-REJECT); `uri_template` (TypedExtensionConfig; PARSE-REJECT). Couples to plugin framework.
- Principal DEFERRED set (3 of 14 per phase 16 SPEC §1.1 amendment 7): `source_ip` (deprecated; PARSE-REJECT envoy-go-only divergence); `metadata` (deprecated; PARSE-REJECT); `custom` (TypedExtensionConfig; PARSE-REJECT — the 14th Principal variant). Couples to plugin framework + mTLS-extension family.
- `Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState` (parse-supported; always-no-match at runtime) — Couples to dynamic-metadata family + filter-state family. Real-world divergence appears only when operator configs explicitly set dynamic-metadata or filter-state from upstream filters.

**Six divergence-windows enumerated at ADR-0146 §Context (the authoritative operator-awareness map):**

1. **LOG-action `access_log_hint` dynamic-metadata divergence-window (per phase 16 SPEC §1.1 amendment 5 + §8.6):** Envoy v1.37.2 sets the `access_log_hint` dynamic metadata key under namespace `envoy.common` to `true` on LOG-matched requests (false on no-match). envoy-go MVP silent-no-metadata-emit. Counter emission: LOG-matched requests increment `allowed` counter (NOT a separate `logged` counter — per phase 16 SPEC §1.1 amendment 8 NO `logged` counter exists in Envoy v1.37.2). Operator divergence-window: dashboards inspecting `access_log_hint` see Envoy emit but envoy-go absent. Future re-activation: dynamic-metadata framework phase lands `EncoderFilterCallbacks.SetDynamicMetadata(key, value)` primitive (or equivalent decode-side accessor).

2. **`response_code_details` field-emission divergence-window (per phase 16 SPEC §1.1 amendment 11 + §8.12):** Envoy v1.37.2's RBAC denial sets `response_code_details = "rbac_access_denied_matched_policy[<sanitized_policy_id>]"` per `utility.cc::responseDetail` (whitespace in policy-id replaced with underscores). The string lands in HCM `response_flag_details` accessor + access-log `RESPONSE_CODE_DETAILS` operator. envoy-go MVP does NOT thread response-code-details from filter through HCM to access-log; current phase-04 HCM scope. Operator divergence-window: access-log `RESPONSE_CODE_DETAILS` field is populated on Envoy-side RBAC denials + empty on envoy-go-side. Future re-activation: response-code-details framework phase couples HCM's local-reply path to a per-filter accessor (`DecoderFilterCallbacks.SetResponseCodeDetails(string)` or analogous).

3. **CEL three-field divergence-window (per phase 16 SPEC §1.1 amendment 6 + Q7):** Policy `condition` (Expr) + `checked_condition` (CheckedExpr) + `cel_config` (CelExpressionConfig) all silent-ignored at runtime. envoy-go MVP folds CEL-aware policies into permissions-OR-principals-only evaluation. Operator divergence-window: configs relying on CEL `condition` for fine-grained gating see Envoy enforce but envoy-go skip. Future re-activation: CEL framework phase lands `internal/cel/` + `github.com/google/cel-go`.

4. **Shadow access-log integration divergence-window (per phase 16 SPEC §8.7 + §11.P13):** envoy-go MVP emits shadow counters only; no shadow-decision-annotated access-log entries. Reference Envoy v1.37.2 confirmed counter-only via source review at SPEC time; no current divergence. Future Envoy version may add access-log integration; impl-time PROGRESS review checks.

5. **SourcedMetadata + FilterState always-no-match runtime divergence-window (per phase 16 SPEC §11.P15 + §8.10):** `Permission_SourcedMetadata` + `Principal_SourcedMetadata` + `Principal_FilterState` parse-supported (no error at config load) but evaluator always returns FALSE at runtime. envoy-go MVP cannot evaluate dynamic-metadata or filter-state predicates because the framework has no metadata-emit or filter-state primitives. Operator divergence-window: configs whose policies depend on dynamic-metadata or filter-state see Envoy evaluate but envoy-go always-FALSE-on-that-rule (potentially deny when Envoy would allow, or vice versa). Future re-activation: dynamic-metadata + filter-state framework phases.

6. **Principal_Authenticated canonical 3-cert-field scope divergence-window (per phase 16 SPEC §1.1 amendment 12 + §11.P14 + ADR-0144 §D11):** envoy-go MVP's `DownstreamPrincipal()` returns URI SAN + DNS SAN + Subject DN CN only. Envoy v1.37.2 also supports Issuer DN, certificate serial number, and fingerprint candidates per the full StringMatcher iteration in `principal.cc::matchPeer`. Operator divergence-window: configs whose `Principal_Authenticated.principal_name` matches against Issuer DN / Serial / Fingerprint see Envoy match but envoy-go no-match. Future re-activation: TLS-context-extension phase widens the accessor's return list.

**Principal_Authenticated nil-principal_name semantic (per phase 16 SPEC §1.1 amendment 12 + §11.P14):** `Principal_Authenticated.principal_name == nil` matches ANY downstream user that passed TLS verification. envoy-go MVP implements three-case algorithm per §6.6: (a) nil principal_name → check `len(DownstreamPrincipal()) > 0`; (b) non-nil → StringMatcher iteration over URI SAN/DNS SAN/Subject DN candidates in priority order; (c) plaintext connection → always FALSE.

**TWO new framework primitives (per phase 16 SPEC §3.1 + §3.2 + ADR-0142 + ADR-0144):** Phase 16 is the FIRST §9 row since phase 14 to introduce non-zero framework deltas + FIRST single phase to introduce TWO: (i) `DecoderFilterCallbacks.DownstreamPrincipal() []string` accessor surfacing the downstream client cert's URI SAN + DNS SAN + Subject DN CN in priority order; (ii) matcher-engine evaluator framework primitive at new top-level `internal/matcher/` package implementing `xds.type.matcher.v3.Matcher` generic match-tree evaluator with PARSE-REJECT-for-unknown-TypeURL discipline. Both cross-phase reusable by future filters (jwt_authn, ext_authz, ext_proc, oauth2).

**ADR-0147 — unanticipated TLS-layer mTLS-lift surfaced at Task 13 follow-up (per Task 13 mTLS-fixture-PKI integration):** Phase-16 fixture 0018 scenario 6 requires a properly mTLS-configured downstream listener (server cert + trusted_ca for client-cert verification + URI SAN `spiffe://example.com/admin` on the test client cert). Phase-03 TLS-layer had a blanket-rejection at config-load of `require_client_certificate=true` (deferred per phase-03 SPEC). ADR-0147 LIFTS that blanket-rejection SCOPED to well-formed mTLS configs (validation_context.trusted_ca PEM provided) — maps onto stdlib `crypto/tls.RequireAndVerifyClientCert` mode + `ClientCAs` pool populated from the parsed trusted_ca. This was unanticipated at phase-16 PLAN time; surfaced at impl-time per ADR-0044 ADR-on-impl convention.

**`track_per_rule_stats` operator-config-driven foot-gun (per phase 16 SPEC §1.1 amendment 8 + §8.5):** When `track_per_rule_stats: true`, the per-policy counter family is allocated lazily on first-match per policy. Misconfigured large-N policy configs (1000+ policies × 2 base sides × 2 (primary + shadow) = 4000 counters per filter instance) impose memory + CPU costs. envoy-go MVP imposes NO parse-time N-cap (mirrors Envoy permissive discipline). Future operator-ergonomics phase MAY add an envoy-go-only N-cap (e.g., max 256 policies under track-true).

**`Principal_Set` + `Permission_Set` recursion depth foot-gun (per phase 16 SPEC §11.P11):** envoy-go MVP imposes NO parse-time recursion-depth cap. Operators authoring deeply-nested rules-engine configs may hit Go-stack-depth issues at config-load time (Go default ~10K frames). Documented above. Future operator-ergonomics phase MAY add an envoy-go-only depth-cap (e.g., max 32 levels of nesting).

**No new tag-extractor (per phase 16 SPEC §1.1 amendment 9 + §11.P7 RATIFIED at Task 8 empirical scrape):** envoy-go's SN2-reuse hypothesis was RATIFIED at Task 8 — `http.<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` renders via the existing SN2 (`http.*` segment routing) + dot→underscore default-branch flatten. NO labels / NO new SN10 rule. ADR-0145 codifies the ratification; the per-policy counter family's `.policy.` infix segment was refined at the same empirical scrape (REFINES SPEC §13.2 stub which omitted the segment).

**Filter-chain ordering with respect to header_mutation / buffer / compressor / bandwidth_limit (per phase 16 SPEC §2.12):** rbac is recommended EARLY in the HCM chain (immediately after listener filters); denied requests don't incur downstream filter cost. Operators wanting header_mutation BEFORE rbac (e.g., to set `X-User` from upstream metadata before the policy gate evaluates it) have full flexibility per the operator's filter-chain order. Fixture 0018 pins rbac as the first HCM filter for byte-equivalence simplicity; SPEC documents the trade-off without prescribing.

### Phase 17 forward-pointer notes

**Deferred items — 17 deferrals + 1 foot-gun, organized by deferred-cluster family** (per phase 17 SPEC §8 + §13.4; silent-ignored or PARSE-REJECT per ADR-0040 + the ADR-0148..ADR-0155 roster; see `### envoy.filters.http.jwt_authn ### Field decomposition` above for the full consumed-vs-ignored field map):

- **dynamic-metadata family (items 1-4):** `payload_in_metadata` + `header_in_metadata` + `failed_status_in_metadata` + `normalize_payload_in_metadata` — all silent-ignored at parse + runtime. envoy-go MVP has no dynamic-metadata-emit framework primitive. Couples to the dynamic-metadata family phase (joint with phase-16 rbac `access_log_hint`). Re-activation lands a `DecoderFilterCallbacks.SetDynamicMetadata(ns, key, value)` primitive.
- **algorithm-extension family (items 5-7, + PS family per planner-time decision 6):** HS256/384/512 (symmetric) + EdDSA + `none` + the RSASSA-PSS `PS256/384/512` family — all DEFERRED. `jwt.Token.VerifySignature` returns `ErrJwtHeaderNotImplementedAlg` for any of them. `none` is PERMANENT-DEFERRED (accepting unsigned tokens is a security anti-pattern). The HS family couples to a shared-secret config-surface phase; EdDSA + PS couple to an algorithm-extension phase.
- **caching-framework family (items 8 + 11):** `jwt_cache_config` (the validated-JWT LRU cache) is silent-ignored — each request re-validates; the `jwt_cache_hit` + `jwt_cache_miss` counters are STRUCTURALLY UNREACHABLE under MVP (registered, never incremented). JWKS cache-invalidation hooks (forced refresh on signature-verification failure) are also deferred. High-RPS performance divergence-window vs reference Envoy (which caches). Re-activation lands an LRU keyed by the raw token string.
- **claim-coverage-extension family (items 15-17):** the v1.37.x JwtProvider fields `subjects` (StringMatcher gate on the `sub` claim) + `require_expiration` (mandate `exp` presence) + `max_lifetime` (cap `exp - iat`) are silent-ignored. `jwt.ValidateOptions` carries the three fields at the API surface but never enforces them. Couples to a claim-coverage-extension phase.
- **filter-state-family (item 12):** `filter_state_rules` is silent-ignored — envoy-go MVP has no `StreamInfo.FilterState` primitive, so it cannot drive runtime requirement-selection off filter-state values. Couples to the filter-state-family phase (joint with phase-16 rbac `Principal_FilterState`).
- **response-code-details family (item 13):** envoy-go MVP does NOT emit `response_code_details` on deny; reference Envoy emits `jwt_authn_access_denied{<failure_reason_with_spaces_as_underscores>}`. Joint pickup with phase-16 rbac forward-pointer item 8 (`rbac_access_denied_matched_policy[...]`) at a future response-code-details framework phase that couples the HCM local-reply path to a per-filter `SetResponseCodeDetails(string)` accessor.
- **access-log family (item 14):** the jwt_authn access-log operators `%JWT_PROVIDER%` + `%JWT_SUBJECT%` + `%JWT_FAILURE_REASON%` are not emitted (no provider-name / subject / failure-reason annotation threaded into the access-log record). Couples to an access-log-extension phase.
- **CEL family (item 10):** CEL-based dynamic provider selection (`requires` driven by a CEL expression over request attributes) is deferred. Joint with phase-16 rbac's CEL three-field deferral; couples to a CEL framework phase landing `internal/cel/` + `github.com/google/cel-go`.
- **operator-ergonomics family (item 18):** the `clear_route_cache` implicit-on-side-effect trigger — Envoy ALSO clears the route cache when `claim_to_headers` adds ≥1 header OR `payload_in_metadata` is set; envoy-go MVP honors only an explicit `clear_route_cache: true`. Exponential-backoff customization for failed JWKS refetch is also deferred (envoy-go MVP uses a fixed 1s interval per §11.P4). Couples to an operator-ergonomics phase.

**Counter divergence-window — `allowed` on bypass paths (discovered at the Task-13 fixture-0019 empirical scrape):** reference Envoy v1.37.2 increments the `allowed` counter on EVERY request that clears the filter gate, including the CORS-preflight-bypass path and the per-route `disabled: true` passthrough. envoy-go MVP increments `allowed` ONLY on an active-engine ALLOWED result, per phase 17 SPEC §3 ("increments per request where the active engine result = ALLOWED") + §1.1 amendment 5 ("`PerRouteConfig{disabled: true}` → no counter increments"). This is SPEC-mandated envoy-go behaviour, NOT a bug — fixture 0019 asserts `allowed` per-side (ref 5 / subj 3) rather than cross-side. Operator divergence-window: dashboards summing `allowed` see a higher count on Envoy than on envoy-go when CORS-preflight or per-route-disabled traffic is present.

**`strip_failure_response` divergence-window (per §11.P3):** when `strip_failure_response: true`, the deny-path SendLocalReply emits an EMPTY body AND NO WWW-Authenticate header (both stripped). envoy-go MVP mirrors Envoy verbatim. Operators relying on the WWW-Authenticate challenge for client-side retry logic must leave `strip_failure_response` unset.

**foot-gun — JwtRequirement Set recursion depth (per phase 17 SPEC §13.4):** envoy-go MVP imposes NO parse-time recursion-depth cap on nested `requires_any` / `requires_all` combinators (mirrors Envoy's permissive disposition). Operators authoring deeply-nested requirement trees may hit Go-stack-depth issues at config-load time. Future operator-ergonomics phase MAY add an envoy-go-only depth-cap (mirrors the phase-16 rbac `Principal_Set` / `Permission_Set` recursion-depth foot-gun).

**TWO new framework primitives (per phase 17 SPEC §3.1 + §3.2 + ADR-0150 + ADR-0151):** phase 17 is the SECOND CONSECUTIVE §9 row (after phase 16 rbac) to introduce two framework primitives in a single phase: (i) `internal/jwks/` HTTP-outbound JWKS fetcher with thread-safe cache + background refresh (per ADR-0150; see `## JWKS framework primitive` above); (ii) `internal/jwt/` pure-Go-stdlib JWS/JWT parser + signature verifier + claim validator (per ADR-0151; see `## JWT verifier framework primitive` above). Both live OUTSIDE `internal/filter/` and are cross-phase reusable — future `ext_authz` HTTP-mode + `oauth2` token-endpoint flows compose against `internal/jwks/Fetcher`; future `jwt_claim_router` + `oauth2` token validation compose against `internal/jwt`.

**8th canonical per-route pattern (per ADR-0125 §(xiii)):** phase 17 is the FIRST §9 row to use the **string-reference-delegation** per-route discipline — `PerRouteConfig{oneof{disabled(bool) | requirement_name(string)}}`. The per-route entry does NOT carry its own filter config; it references-by-name into the listener-level `requirement_map`. Dangling `requirement_name` references are RUNTIME-RESOLVED (mirrors Envoy `filter_config.cc findPerRouteVerifier` — emits 403 + `"Failed JWT authentication: Wrong requirement_name: <name>"` on miss), NOT parse-rejected. Per-route stats are SHARED with listener-level (pure delegation spawns no new policy-evaluation state). ADR-0125's canonical-pattern roster grows from 7 to 8.

**No new tag-extractor (per §11.P7 RATIFIED at the Task-13 fixture-0019 empirical scrape):** envoy-go's SN2-reuse hypothesis was RATIFIED — `http.<HCM_stat_prefix>.jwt_authn.<counter>` renders via the existing SN2 (`http.*` segment routing) + dot→underscore default-branch flatten as `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="<HCM_stat_prefix>"}`. Both reference Envoy v1.37.2 and envoy-go emit the identical Prometheus form. NO new SN10 rule; NO new tag-extractor. ADR-0154 §Decision (vi) codifies the ratification.

### Phase 18.1 forward-pointer notes

**Deferred items — 11 deferrals + 1 joint divergence-window, organized by deferred-cluster family** (per phase 18.1 SPEC §8 + parent SPEC §8; silent-ignored or PARSE-REJECT per ADR-0040 + the ADR-0156..ADR-0163 roster; see `### envoy.filters.http.ext_authz ### Field decomposition` above for the consumed-vs-deferred field map):

- **gRPC service mode (item 1):** `grpc_service` arm PARSE-REJECTs in 18.1 with `"ext_authz: grpc_service mode not yet supported (lands in phase 18.2)"`. Lands in **phase 18.2** — the `internal/grpcclient/` gRPC-client framework primitive (ADR-0158), the `grpc_service` arm activated in the `compiledConfig` dispatch (ADR-0157 §Decision amended), the gRPC-mode `AttributeContext`/`CheckRequest` builder (ADR-0160 gRPC-mode portion), and the `CheckResponse` → disposition mapping including `OkHttpResponse`/`DeniedHttpResponse` header mutation (ADR-0161 gRPC-mode portion). This is not a deferral so much as a sequenced split (18.2 is the explicit next sub-phase).

- **Dynamic-metadata family (item 2):** the four `*metadata_context_namespaces` fields + `AuthorizationResponse.dynamic_metadata_from_headers` + `enable_dynamic_metadata_ingestion` + `filter_metadata` — all SILENT-IGNORED at parse + runtime. envoy-go MVP has no dynamic-metadata-emit framework primitive. ext_authz is the THIRD §9 filter blocked on this family (joint with phase-16 rbac `access_log_hint` + phase-17 jwt_authn `payload_in_metadata` etc.). Re-activation: dynamic-metadata family phase lands `DecoderFilterCallbacks.SetDynamicMetadata(ns, key, value)`.

- **`filter_enabled` / `filter_enabled_metadata` / `deny_at_disable` (item 4):** Runtime family + matcher/metadata family. All three default to no-op when unset (per parent §5.P12), so 18.1 fixture configs need NO explicit settings. Consequence: the `disabled` counter is STRUCTURALLY UNREACHABLE under MVP (parent §6 amendment 7 — the runtime `filter_enabled` gate would be its trigger; envoy-go always-evaluates). Re-activation: Runtime family phase brings `RuntimeFractionalPercent` + `MetadataMatcher` support.

- **`allowed_client_headers_on_success` (item 5):** DEFERRED per parent §5.P9 + §6 amendment 9. The decode-side-only filter shape has no encode leg — copying auth-response headers to the downstream RESPONSE on the allow path requires either an encode-side leg (ADR-0161 §Consequences revisit note) or a HCM stash mechanism. Divergence-window: reference Envoy v1.37.2 supports this; envoy-go MVP silently ignores it. Re-activation: encode-side leg framework phase.

- **`charge_cluster_response_stats` + cluster-scoped stat triple (item 6):** `cluster.<upstream>.ext_authz.{ok,denied,error}` stat triple DEFERRED per parent §6 amendment 8 — a NEW stat-namespace pattern (charging into the cluster stat tree, not the HCM stat tree). Re-activation: a future cluster-stat-charging phase extends the stat surface.

- **`emit_filter_state_stats` + `bootstrap_metadata_labels_key` + `decoder_header_mutation_rules` (item 7):** `emit_filter_state_stats` couples to the filter-state/access-log family; `bootstrap_metadata_labels_key` couples to node-metadata-labels; `decoder_header_mutation_rules` is the per-rule mutation-rejection surface (distinct from the MVP `validate_mutations` correctness checks — `validate_mutations` IS consumed). All SILENT-IGNORED.

- **`OkHttpResponse.query_parameters_to_set` / `query_parameters_to_remove` (item 3):** DEFERRED — path-query rewriting subsystem (ADR-0112; joint with phase-10 header_mutation's `query_parameter_mutations`). NOTE: `OkHttpResponse` is gRPC-mode; the analogous HTTP-mode concern does not arise in 18.1. Re-activation: path-query rewriting phase lands `OkHttpResponse`→`:path` mutation.

- **`context_extensions` HTTP-mode no-op (item 8):** `CheckSettings.context_extensions` is a `map[string]string` field documented in the proto as gRPC-mode-only (carries request attributes to the gRPC auth server via `AttributeContext.context_extensions`). In 18.1 HTTP-mode, it PARSES but has no HTTP-mode effect. Re-activation: when 18.2 activates the gRPC arm, the same `context_extensions` map is forwarded in the gRPC `CheckRequest.attributes.context_extensions`.

- **`response_code_details` emission — `ext_authz_denied` (item 10):** DEFERRED joint divergence-window with phase-16 rbac (`rbac_access_denied_matched_policy[...]`) + phase-17 jwt_authn (`jwt_authn_access_denied{...}`) — envoy-go MVP does NOT emit `response_code_details` on deny (phase-04 HCM does not surface `response_code_details` to local-reply callers). Reference Envoy v1.37.2 emits `ext_authz_denied` on the deny path. This is the THIRD §9 filter contributing to the `response_code_details` joint deferred-cluster. Re-activation: response-code-details framework phase couples HCM local-reply path to a per-filter `SetResponseCodeDetails(string)` accessor.

- **Access-log integration (item 11):** ext_authz decision fields (`%EXT_AUTHZ_*%`-style formatters) are not emitted. Couples to an access-log-extension framework phase (joint with phase-16/17).

- **`internal/httpclient/` generalization forward-pointer (from ADR-0159 disposition (b)):** Phase 18.1 keeps a thin ext_authz-local HTTP client rather than generalizing into a shared `internal/httpclient/` package (two consumers whose lifecycles barely overlap). The natural trigger is the THIRD outbound-HTTP consumer — anticipated `oauth2` token-endpoint flows (synchronous-per-request POST; same shape as ext_authz's auth-check). When oauth2 brainstorms, the `internal/httpclient/` generalization should be reconsidered with three consumers in view. See `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)` above.

**`response_code_details` joint divergence-window (phases 16 + 17 + 18.1):** Three §9 filters now contribute to this joint deferred cluster — rbac (`rbac_access_denied_matched_policy[...]`), jwt_authn (`jwt_authn_access_denied{...}`), ext_authz (`ext_authz_denied`). All three envoy-go MVP implementations do NOT emit `response_code_details`. The forward-pointer is now CUMULATIVE — a single response-code-details framework phase would close all three simultaneously.

**No new ADR-0125 canonical pattern — 5th-canonical-REUSE (per ADR-0163):** Phase 18.1 is the FIRST §9 family-row since phase 13 to REUSE an existing ADR-0125 canonical rather than extend the roster (breaking the phase-13-§(ix) / phase-14-§(x) / phase-15-§(xi) / phase-16-§(xii) / phase-17-§(xiii) per-phase-roster-growth streak). ADR-0125's canonical-pattern roster STAYS at 8 entries after phase 18.1. The 5th canonical (disabled-bool arm + NARROWER override sub-message arm in a required oneof) now has THREE consumers: buffer (phase 13) + compressor (phase 14) + ext_authz (phase 18.1). See `## Per-route canonical patterns cross-reference` below for the updated table. (NOTE: the `grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md = 0` authoritative check confirms NO §(xiv) amendment paragraph exists — the three explanatory-text matches for `\(xiv\)` in DECISIONS.md all describe the ABSENCE of §(xiv), not a new canonical.)

**No new framework primitive from 18.1 (composed-against existing primitives):** Phase 18.1 introduces ONE new one-way framework primitive — the per-request HTTP-outbound auth-check POST in `check.go` (ADR-0159; see `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)` above) — composing against phase-09 async-resume + phase-13 ADR-0128 body-buffering + phase-14 ADR-0131 `OverwriteBody` (anticipated NOT invoked) + phase-17 `internal/jwks/Fetcher` outbound-HTTP structure. This is NOT a new SHARED cross-phase primitive (unlike phase-16's `internal/matcher/` + phase-17's `internal/jwks/` + `internal/jwt/`) — it is ext_authz-local per ADR-0159 disposition (b). Phase 18.2 adds the gRPC-client `internal/grpcclient/` primitive (ADR-0158 — the FIRST in-process gRPC infrastructure in the envoy-go test tree).

### Phase 18.2 forward-pointer notes

**Deferred items — 12 deferrals (the 11 phase-18.1 carry-forwards + the 5 new gRPC-specific items minus 2 closures) + 1 envoy-go-strict exclusion, organized by deferred-cluster family** (per phase 18.2 SPEC §8 + parent SPEC §8 + 18.1 SPEC §8 carry-forward; silent-ignored or PARSE-REJECT per ADR-0040 + the ADR-0158 / ADR-0165 / ADR-0166 trio added at 18.2; see `### envoy.filters.http.ext_authz` above for the full populated/consumed/deferred map). 18.2 CLOSES 2 phase-18.1 forward-pointers: (a) item 1 "gRPC service mode" (sequenced split — now landed) + (b) item 8 "`context_extensions` HTTP-mode no-op" (now consumed proto-faithful in gRPC mode).

- **18.1 carry-forwards UNCHANGED (the 11 items per 18.1 SPEC §8 minus the 2 closed by 18.2):** `*metadata_context_namespaces` (4 fields silent-ignored), `filter_enabled` family (3 fields + the structurally-unreachable `disabled` counter), `enable_dynamic_metadata_ingestion` + `filter_metadata` (dynamic-metadata family), `charge_cluster_response_stats` + the cluster-scoped stat triple (`cluster.<upstream>.ext_authz.{ok,denied,error}` — deferred per parent §6 amendment 8; couples to a future cluster-stat-charging phase), `emit_filter_state_stats` + `bootstrap_metadata_labels_key` + `decoder_header_mutation_rules` (filter-state / node-metadata / per-rule-mutation families), `allowed_client_headers_on_success` (decode-side-only filter shape; no encode leg), `response_code_details` emission (joint with phases 16 + 17 + 18.1), access-log integration (`%EXT_AUTHZ_*%`-style formatters not emitted), `internal/httpclient/` generalization forward-pointer (UNCHANGED — the 18.2 gRPC-client primitive at `internal/grpcclient/` is a separate cross-phase primitive; the HTTP-outbound generalization trigger remains the THIRD outbound-HTTP consumer per ADR-0159).

- **`core.GrpcService.initial_metadata` (gRPC-specific item 2):** SILENT-IGNORED per 18.2 SPEC §2.6 + §8 item 2. This is a `[]HeaderValue` of metadata-pairs to send on every Check call (gRPC-mode equivalent of a request-header bundle). MVP does NOT thread these into the per-Check context — `*authClient.Check(ctx, *CheckRequest)` is invoked without `metadata.NewOutgoingContext` wrapping. Re-activation: a future gRPC-client-metadata phase lands the per-call metadata plumbing on `*grpcclient.AuthClient`.

- **`core.GrpcService.retry_policy` (gRPC-specific item 3):** SILENT-IGNORED per 18.2 SPEC §8 item 3. gRPC client retry is a follow-up; the current MVP single-attempt-then-error matches 18.1's `httpAuthClient` zero-retry discipline + the parent §5.P10 error-classification boundary. Re-activation: a future retry-framework phase folds both the HTTP-mode + gRPC-mode retry paths together.

- **`core.GrpcService_GoogleGrpc` (envoy-go-strict exclusion; item 4 — NOT a deferral):** PARSE-REJECT envoy-go-strict per 18.2 SPEC §6.5 step 1 + parent §4.3 + ADR-0008 V3-only-transport-discipline. envoy-go uses `google.golang.org/grpc` directly; the `GoogleGrpc` arm (which configures a self-contained gRPC client with its own dial/TLS/retry semantics) is permanently out-of-scope for envoy-go. Listed here for surface-completeness; this is NOT a re-activation surface.

- **`OkHttpResponse.response_headers_to_add` (gRPC-specific item 5):** SILENT-IGNORED / DEFERRED per 18.2 SPEC §8 item 5. The decode-side-only filter shape has no encode leg — copying auth-response headers to the downstream RESPONSE on the allow path requires either an encode-side leg or a HCM stash mechanism. Joint divergence-window with the 18.1 carry-forward `allowed_client_headers_on_success` (same family). Re-activation: encode-side leg framework phase OR HCM-stash-for-response-mutation primitive.

- **`OkHttpResponse.query_parameters_to_set` / `query_parameters_to_remove` (gRPC-specific item 6):** DEFERRED — path-query rewriting subsystem (ADR-0112; joint with phase-10 header_mutation's `query_parameter_mutations` + 18.1 carry-forward of the analogous HTTP-mode concern). Re-activation: path-query rewriting phase lands `OkHttpResponse`→`:path` mutation.

- **`OkHttpResponse.dynamic_metadata` + `CheckResponse.dynamic_metadata` (gRPC-specific item 7):** DEFERRED — dynamic-metadata family, joint with phases 16 + 17 + 18.1 carry-forward. envoy-go MVP has no dynamic-metadata-emit framework primitive. Re-activation: dynamic-metadata framework phase lands `DecoderFilterCallbacks.SetDynamicMetadata(ns, key, value)`.

- **`encode_raw_headers: true` (the `header_map` arm) (gRPC-specific item 8 — CONDITIONALLY DEFERRED per D6):** the flag PARSES; the `header_map` field is NOT populated when set true under MVP (the legacy `headers` map populates regardless). Both Envoy and envoy-go produce IDENTICAL `headers` maps when `encode_raw_headers: false` (the default). Re-activation: a future enhancement when an operator-visible behavior gap surfaces (an auth-service operator relying on per-header-name order or duplicate-key preservation in the proto would observe this gap).

- **xDS-CDS-driven auth-cluster reconfig (gRPC-specific item 9):** DEFERRED — envoy-go has no xDS-CDS yet (static config only). The `*grpcclient.AuthClient` lifecycle is tied to the static `compiledConfig`; when xDS-CDS lands, hot-replacement gains a close-on-replacement discipline. The MVP leaks-on-process-exit posture is per `*grpc.ClientConn` (no explicit `Close()` call; matches Envoy's gRPC-client lifecycle convention). Re-activation: xDS / dynamic config family phase.

- **TLS-fronted gRPC auth cluster fixture coverage (gRPC-specific item 10):** DEFERRED. The §11.P13 in-session SPEC scrape RATIFIED the TLS-to-auth-cluster path; fixture 0021 uses plaintext h2c for the auth cluster to keep the fixture topology simple. A future integration test MAY extend coverage if a behavior gap surfaces. Note: ADR-0166 lifts the prior TLS-required gate on h2c upstream clusters (cluster-manager amendment landing in the same phase) — TLS auth-cluster fixture coverage is a future enhancement, NOT a re-activation of a previously-disabled surface.

**Two 18.1 forward-pointers CLOSED at 18.2:**

- **18.1 item 1 — gRPC service mode:** CLOSED. The `grpc_service` arm activated in `buildCompiledConfig` calls `buildGRPCCheckFn`; the `internal/grpcclient/` framework primitive (ADR-0158) lands; ADR-0157 §Decision is amended in-place at the 18.2 IMPL anchor (Task 3); ADR-0160 + ADR-0161 grow gRPC-mode portions at Tasks 5 + 6. The sequenced split is done.
- **18.1 item 8 — `CheckSettings.context_extensions` HTTP-mode no-op:** CLOSED for gRPC mode. The merged listener+per-route `context_extensions` map populates `AttributeContext.context_extensions` per ADR-0160 gRPC-mode portion. The HTTP-mode "no-op" framing remains accurate for the HTTP service mode (the field has no HTTP-mode effect by proto design); the consumption in gRPC mode closes the joint deferral.

**Callback-surface extension (per ADR-0165 — the ADR-0044 escape-valve fired):** Phase 18.2 lands **6 new methods on `envoyhttp.DecoderFilterCallbacks`** at IMPL Task 4 — `DownstreamRemoteAddr() net.Addr`, `DownstreamLocalAddr() net.Addr`, `DownstreamTLSServerName() string`, `DownstreamTLSPeerCertDER() []byte`, `DownstreamProtocol() string`, `ListenerPrincipal() string`. The PLAN-time D3 + D12 settle: the 18.2 SPEC §13.5 + §6.5 + §6.6 originally pinned "NO new method on `envoyhttp.DecoderFilterCallbacks` lands at 18.2"; at PLAN time the planner re-verified the master-tip callback surface against the SPEC's own §15 acceptance item 4 + §11.P4 RATIFICATION (populated `tls_session.sni` / `source.certificate` / source + destination socket addresses / `destination.principal` / `request.http.protocol`) and determined the populated set is UNSATISFIABLE without callback extension. The PLAN settled by AMENDING §13.5 + §6.5 step 5 + §6.6 in-place at Task 4. The 6 methods anchor a cross-phase-reusable framework primitive (consumed by ext_proc + global_ratelimit + future ext_authz extensions). The behaviorally-significant divergence the original §13.5 would have produced (UNPOPULATED `tls_session.sni` + `source.certificate` + `destination.principal` + socket addresses + `request.http.protocol`) is avoided. See ADR-0165 §Context for the falsified-at-PLAN-time analysis + cross-phase reuse rationale + the `## HTTPFilterCallbacks` section above for the per-method documentation. **Cross-phase reuse intent:** ext_proc (next §9 row OR a future phase), global_ratelimit, ext_authz extensions for non-MVP fields all consume the same 6 accessors.

**Cluster-manager amendment — plaintext h2c upstream relaxation (per ADR-0166 — unanticipated, fixup-time):** Phase 18.2 Task 11.5 (a fixup commit landing between Task 11 and Task 12 per the §11.5 sub-task numbering convention) lifts the prior TLS-required gate on h2c upstream clusters in `cluster.Manager.extractH2Mode` / `dial_h2.go`. Rationale: fixture 0021's three-listener topology requires a plaintext h2c auth-cluster to keep the in-process gRPC test-helper simple (no TLS PKI scaffolding required); the previous TLS-required gate rejected plaintext h2c upstream clusters at config-load time. ADR-0166 records the gate relaxation + the plaintext-h2c upstream-cluster dial path. Cross-phase reuse intent: any future gRPC-bridge / gRPC-Web / gRPC-JSON transcoding / H2-upstream-only filter benefits from the same plaintext h2c dial path. The cluster-manager amendment is NOT a TLS-layer lift — TLS upstream clusters still wrap a TLS `net.Conn`; plaintext upstream clusters now correctly route through the H2-dial path without the TLS-required precondition.

**gRPC client framework primitive — NEW SHARED cross-phase primitive (per ADR-0158):** Phase 18.2 introduces the `internal/grpcclient/` package — envoy-go's FIRST gRPC infrastructure of any kind. Cross-phase reuse intent: ext_proc + global_ratelimit + any future filter that wires a gRPC outbound to an upstream cluster composes against `*grpcclient.Dialer` + a thin typed wrapper for the relevant proto service. See `## gRPC client framework primitive (per phase 18.2 ADR-0158)` below for the full umbrella.

**5 new ADRs at 18.2 (the 18.2-portion of the §11.P13 in-session SPEC RATIFICATION net delta):** ADR-0158 (§Decision + §Consequences — full body; §Context was at parent SPEC commit `308e9b6`); ADR-0157 (§Decision AMENDMENT in-place at Task 3 — `grpc_service` arm activation); ADR-0160 (gRPC-mode portion at Task 5); ADR-0161 (gRPC-mode portion at Task 6); ADR-0165 (callback-surface extension — the ADR-0044 escape-valve fired per planner-time D3 + D12 at Task 4); ADR-0166 (plaintext h2c upstream relaxation — unanticipated fixup-time amendment at Task 11.5). Net delta: 5 §Decision-bodies authored at 18.2 IMPL + 1 in-place AMENDMENT + 1 fixup-time ADR = 7 §Decision-touchpoints; 2 NEW ADR numbers consumed (ADR-0165 + ADR-0166); ADR-0044 escape-valve fired ONCE at PLAN time (D12) + ONCE at IMPL-fixup time (ADR-0166).

**Joint `response_code_details` divergence-window (phases 16 + 17 + 18.1 + 18.2):** UNCHANGED by 18.2 — the gRPC-mode deny path also does NOT emit `response_code_details`. Reference Envoy v1.37.2 emits `ext_authz_denied` on the deny path in both modes. The four §9 filters (rbac, jwt_authn, ext_authz-HTTP, ext_authz-gRPC — but the latter two share the same MVP filter implementation, so this is the SAME divergence-window not a fourth) now all contribute to this joint deferred cluster.

### Phase 19.1 forward-pointer notes

**Deferred items — 18 deferrals + 1 envoy-go-strict exclusion (per phase 19.1 SPEC §8 + parent SPEC §4.4 + §8):**

- **Body-stage activation** (`request_body_mode`/`response_body_mode = BUFFERED`) — DEFERRED to phase 19.2. PARSE-REJECT in 19.1. The 19.2 SPEC lands ADR-0175 (encode-side body-buffering framework primitive — analogous to phase-13 ADR-0128 decode-side; REFUTED at parent §5.P11 SPEC-time scrape → fires load-bearing) + ADR-0168 §Decision AMENDMENT lifting the listener-level body-mode PARSE-REJECT for BUFFERED only + ADR-0171 + ADR-0172 body-mode AMENDMENTS.
- **STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes** — DEFERRED permanently (out of envelope per parent Q2). PARSE-REJECT envoy-go-strict.
- **Trailer modes != SKIP** (`request_trailer_mode` + `response_trailer_mode`) — DEFERRED permanently (out of envelope per parent Q2; couples to a framework-wide trailer-pass-through primitive blocked since phase-04 HTTP/1.1).
- **`observability_mode`** — DEFERRED permanently (STREAMED-only flag; out of envelope per Q2; PARSE-REJECT).
- **`send_body_without_waiting_for_header_response`** — DEFERRED permanently (STREAMED-only flag; PARSE-REJECT).
- **`deferred_close_timeout`** — DEFERRED permanently (STREAMED-only flag; PARSE-REJECT for non-zero).
- **`metadata_options` (forwarding_namespaces + receiving_namespaces)** — DEFERRED (dynamic-metadata family — blocked at phases 16+17+18 forward-pointers; ext_proc now the FOURTH §9 filter blocked on this family).
- **`filter_metadata`** — DEFERRED (same dynamic-metadata coupling).
- **`ProcessingResponse.dynamic_metadata` emit + `ProcessingRequest.metadata_context`** — DEFERRED (dynamic-metadata family; empty-proto-message shape stays for forward-compat).
- **`CommonResponse.dynamic_metadata` and `CommonResponse.trailers`** — DEFERRED (both proto-flagged `[#not-implemented-hide:]` or dynamic-metadata family).
- **`HttpHeaders.attributes`** (deprecated + `[#not-implemented-hide:]`) — silent-ignore (the proto-doc says deprecated and not implemented; envoy-go honors the convention).
- **`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}`** — DEFERRED per ADR-0173 (silent-ignore the three `[#not-implemented-hide:]` fields `async_mode` + `request_attributes` + `response_attributes`; defer the dynamic-metadata + initial-metadata families). The top-level `ExternalProcessor.request_attributes`/`response_attributes` (#5/#6) are MVP-CONSUMED for the listener-level attribute envelope.
- **`core.GrpcService.GoogleGrpc` arm** — envoy-go-strict EXCLUSION (PARSE-REJECT — inherited from ADR-0157 §Decision AMENDMENT; envoy-go uses Go gRPC directly).
- **`core.GrpcService.{initial_metadata, retry_policy}`** — SILENT-IGNORED for MVP (gRPC client retry + initial-metadata follow-ups; joint with phase-18.2 forward-pointer).
- **`response_code_details` emission** (the `ImmediateResponse.details` field) — DEFERRED (envoy-go HCM does not surface response_code_details; joint divergence-window with phases 16+17+18 forward-pointers).
- **xDS-CDS-driven processor-cluster reconfig** — DEFERRED (envoy-go has no xDS-CDS yet; joint with phase-18.2 forward-pointer).
- **TLS-fronted processor-cluster fixture coverage** — DEFERRED (fixture 0022 uses plaintext h2c; mirrors phase-18.2 fixture 0021 disposition; reuses ADR-0166 plaintext h2c relaxation).
- **8 reference-only ext_proc counters (per §19.P4 RATIFIED-WITH-AMENDMENT)** — `immediate_responses_sent`, `message_timeouts`, `clear_route_cache_disabled`, `clear_route_cache_ignored`, `clear_route_cache_upstream_ignored`, `rejected_header_mutations`, `server_half_closed`, `http_not_ok_resp_received` are emitted by reference Envoy v1.37.2 but NOT registered in envoy-go MVP. The fixture-harness assertion gate is relaxed to PRESENCE-check (not delta-equality) on the 9 MVP counters. Phase 19.2 IMPL is the natural activation site for several of these (body-mode wiring touches `immediate_responses_sent`, `message_timeouts`, `rejected_header_mutations`).

**Per-message timer enforcement carry-forward (per ADR-0172 §Decision (vii)):** at 19.1 the per-message `msgCtx` built via `context.WithTimeout(f.streamCtx, perMessageTimeout)` is NOT bound to `stream.Send` / `stream.Recv` (gRPC ClientStream contract — Send/Recv inherit ctx from `Process(ctx)` at stream-open). Per-message timer enforcement is STRUCTURAL-ONLY at 19.1; full pre-emption-on-timeout via `time.AfterFunc(perMessageTimeout, f.streamCancel)` lands at 19.2 alongside the body-stage streaming-aware primitive (the streaming Recv pattern changes the right primitive shape — designed ONCE for 19.2 rather than shipping a 19.1 stop-gap).

**`applyProcessingResponseFn` test-infrastructure cleanup carry-forward (per Task 12 review):** the package-level `var applyProcessingResponseFn` indirection (per ADR-0171 §Decision (ix) — landed at Task 7 for clean Task 7 → Task 8 dispatcher-vs-body handoff) causes test-infrastructure non-parallel discipline (test overrides via `withApplyOverride` t.Cleanup must not race). Future cleanup: promote to a `compiledConfig` field (or `factoryState` field) so each test gets isolated swap without t.Parallel contention. Documented as a 19.2 cleanup carryforward — lower priority; the discipline is locally containable today.

**`internal/jsoncodec/` generalization forward-pointer (per ADR-0170 disposition (b)):** the filter-local `ProcessingRequest`/`ProcessingResponse` JSON codec at `internal/filter/http/extproc/json.go` mirrors the phase-18.1 ADR-0159 (b)-disposition rationale — filter-local until the SECOND consumer trigger. Anticipated second consumer: a future filter consuming a `protojson`-encoded outbound HTTP payload (e.g., a future webhook filter); when that lands, the generalization to `internal/jsoncodec/` should be reconsidered.

**`response_code_details` joint divergence-window (phases 16 + 17 + 18.1 + 18.2 + 19.1):** EXTENDED by 19.1 — the ImmediateResponse deny path also does NOT emit `response_code_details` (envoy-go MVP joint divergence with phases 16/17/18). Reference Envoy v1.37.2 emits `ext_proc_*` family details strings on the deny path. Five §9 filters (rbac, jwt_authn, ext_authz-HTTP, ext_authz-gRPC, ext_proc) now contribute to this joint deferred cluster — strengthens the case for a dedicated `response_code_details` framework phase.

**Dynamic-metadata family joint divergence-window (phases 16 + 17 + 18 + 19.1):** EXTENDED by 19.1 — ext_proc's `metadata_options` + `filter_metadata` + `CommonResponse.dynamic_metadata`-style emit + `ProcessingRequest.metadata_context` join the deferred cluster. ext_proc is now the FOURTH §9 filter blocked on the dynamic-metadata family — strengthens the case for a dedicated dynamic-metadata-family phase.

**Phase-18.2 forward-pointer CLOSED at 19.1:** ADR-0174 closes the **encode-side callback symmetry concern** the phase-18.2 ADR-0165 left open (the 18.2 `DecoderFilterCallbacks` extension added 6 socket/TLS/listener accessors decode-side only; ADR-0174 extends the same 6 methods to `EncoderFilterCallbacks` via the existing chain-field plumbing — NO new chain primitives, NO new HCM-dispatch SET sites). The 18.2 forward-pointer's "encode-side asymmetry concern" no longer applies to any §9 filter consuming encode-side socket/TLS state. Cross-phase reuse intent: future encode-side filters needing socket/TLS introspection (response-validation, encode-side rate-limiting, encode-side header-injection from cert state) consume the same 6 accessors via the existing `*encoderCB` reader methods.

**No new ADR-0044 escape-valve fired at 19.1 IMPL (D12 hypothesis HELD):** the planner-time D12 hypothesis predicted NO impl-time-unanticipated ADR fires at 19.1 IMPL (the SPEC-time scrape closure of §19.P11/§19.P12 removed the most-likely escape-valve surfaces — both ADR-0174 + ADR-0175 fired SPEC-time load-bearing rather than IMPL-time). The IMPL confirms: 8 anticipated ADRs (ADR-0167..ADR-0174) §Decision + §Consequences bodies landed at their per-Task Lands-in-Tasks; ADR-0177 stays unconsumed (reserved for 19.2 + any 19.2-IMPL-unanticipated surface). The 19.1 IMPL closed three RATIFIED-PENDING-IMPL-TIME pins: §19.P4 RATIFIED-WITH-AMENDMENT (9 hypothesized counters all present; 8 reference-only deferred); §19.P7 RATIFIED-BY-CONSTRUCTION (cache-on-first-use established by single resolution entry point); §19.P8 RATIFIED on the codec-options axis (snake_case + omit-zero + enum-as-string protojson options matching reference; three envelope-content divergences deferred to 19.2).

### Phase 19.2 forward-pointer notes

**19.2 closes 2 of the 19.1 forward-pointers** at phase-19.2 phase-done:

- **Body-stage activation for gRPC-service-mode (`request_body_mode = BUFFERED` + `response_body_mode = BUFFERED`)** — CLOSED via ADR-0168 §Decision AMENDMENT (PARSE-REJECT lift) + ADR-0175 NEW encode-side body-buffering primitive + ADR-0171 §Decision AMENDMENT 4-stage state machine + ADR-0172 §Decision AMENDMENT body-mode arms of `applyProcessingResponse`. HTTP-service-mode body-mode PARSE-REJECT continues per the proto's `ExtProcHttpService` constraint (NOT closed by 19.2).
- **19.1 SPEC §12 deferred decision #7 (`CONTINUE_AND_REPLACE` handling)** — SETTLED at 19.2 SPEC per `### envoy.filters.http.ext_proc → #### Body-stage wire shape` SPEC §4.3 table above. Header stages with body-mode = NONE: CONSUMED as no-op (19.1 spurious-dispatch LIFTS); Header stages with body-mode = BUFFERED: CONSUMED as combined header+body replacement; Body stages: TREATED AS CONTINUE.

**Per-message timer enforcement (per ADR-0172 §Decision (vii) carry-forward from 19.1 → ADR-0171 §Decision AMENDMENT at 19.2):** CLOSED at 19.2 IMPL Task 4 — single rolling per-direction timer via `context.WithTimeout` cancel-and-rebuild (planner-time D4); race-tested at IMPL Task 8 (`TestPerMessageTimer_*` group).

**Deferred items carry-forward from 19.1 §8 (17 of 18 items remain DEFERRED at 19.2 phase-done):**

- **STREAMED + BUFFERED_PARTIAL + FULL_DUPLEX_STREAMED body modes** — PERMANENT PARSE-REJECT (out of envelope per parent §4.4).
- **Trailer modes != SKIP** — PERMANENT PARSE-REJECT (couples to framework-wide trailer-pass-through primitive blocked since phase-04 HTTP/1.1).
- **`observability_mode` + `send_body_without_waiting_for_header_response` + `deferred_close_timeout`** — PERMANENT PARSE-REJECT (STREAMED-only flags).
- **HTTP-service-mode body activation** — PERMANENT (proto's `ExtProcHttpService` constraint enforces headers-only).
- **`metadata_options` / `filter_metadata` / `ProcessingResponse.dynamic_metadata` emit / `ProcessingRequest.metadata_context` / `CommonResponse.dynamic_metadata` / `CommonResponse.trailers`** — DEFERRED (dynamic-metadata family — joint divergence-window with phases 16+17+18 forward-pointers).
- **`HttpHeaders.attributes`** — silent-ignore (proto-flagged deprecated + `[#not-implemented-hide:]`).
- **`ExtProcOverrides.{async_mode, request_attributes, response_attributes, metadata_options, grpc_initial_metadata}`** — DEFERRED per ADR-0173 (the per-route `request_attributes` / `response_attributes` at #3/#4 are flagged `[#not-implemented-hide:]` and distinct from the MVP-CONSUMED top-level `ExternalProcessor.request_attributes` / `response_attributes` at #5/#6).
- **`core.GrpcService.GoogleGrpc`** — envoy-go-strict EXCLUSION (PARSE-REJECT — inherited from ADR-0157 §Decision AMENDMENT).
- **`core.GrpcService.{initial_metadata, retry_policy}`** — SILENT-IGNORED for MVP.
- **`response_code_details` emission** (`ImmediateResponse.details`) — DEFERRED (joint divergence-window with phases 16+17+18+19.1 — see below).
- **xDS-CDS-driven processor-cluster reconfig** — DEFERRED (envoy-go has no xDS-CDS yet).
- **TLS-fronted processor-cluster fixture coverage** — DEFERRED (reassigned to the TLS-fixture phase per phase-19.2 SPEC §2 item 7; mirrors phase-18.2 fixture 0021 + phase-19.1 fixture 0022 plaintext h2c disposition; reuses ADR-0166 relaxation).
- **8 reference-only ext_proc counters (per §19.P4 RATIFIED-WITH-AMENDMENT carry-forward at 19.2)** — `immediate_responses_sent` / `message_timeouts` / `clear_route_cache_disabled` / `clear_route_cache_ignored` / `clear_route_cache_upstream_ignored` / `rejected_header_mutations` / `server_half_closed` / `http_not_ok_resp_received` — DEFERRED to phase 19.3+ activation per phase-19.2 SPEC §2 item 5 (HOLDS at 9 counters / 86-stat-name BEHAVIOR_CONTRACT total at 19.2 phase-done).
- **3 ADR-0170 §Consequences envelope-content divergences** — protojson whitespace non-determinism; `metadata_context:{}` + `protocol_config:{}` empty-message emission; writer-side `value`-vs-`raw_value` on header injection — DEFERRED to a future phase (body-mode mechanics orthogonal to envelope-content rendering).
- **`mode_override` body-response-path responsiveness** — per parent §5.P1 RATIFIED-AND-REFINED — `mode_override` on body-stage responses silently dropped (NOT spurious); REFINEMENT stays UNCHANGED at 19.2.
- **`ExtProcOverrides.processing_mode` body-mode arms for HTTP-service-mode** — DEFERRED (paralleling HTTP-service-mode body-mode PARSE-REJECT). For gRPC-service-mode the per-route body-mode arms are CONSUMED at 19.2 per per-route 5th-canonical REUSE (`### Per-route discipline → per-route 5th-canonical REUSE`).
- **`request_attributes` / `response_attributes` CEL-attribute-name exact roster** — IMPL-settle continues at 19.2; the body-stage envelope MIRRORS the header-stage roster + adds body-stage-natural `request.size`/`response.size` (D5 disposition HOLDS at fixture-0023 scrape per PROGRESS Task 9).

**NEW 19.2-specific empirical-pin AMENDMENT forward-pointers** (captured at fixture-0023 scrape per PROGRESS Task 9; future-phase closure surfaces):

- **(I) Reference Envoy v1.37.2 body-stage `body_mutation` returns 500** when the processor emits `CommonResponse{body_mutation{body|clear_body}}` at the response_body stage under `response_body_mode: BUFFERED`. envoy-go correctly applies the mutation (unit-tested at IMPL Task 6 + race-tested at IMPL Task 8; cross-side fixture-0023 scenarios (b)+(d) re-scoped to OBSERVABILITY-only). Root-cause analysis + closure deferred to a future phase. The substantive cross-side byte equivalence at the differential gate HOLDS (both sides see the original upstream body unchanged in the re-scoped scenarios).
- **(II) envoy-go HCM encode-side `SendLocalReply` framework gap** — HCM rejects encode-side `SendLocalReply` after the encode chain has started (log line `"hcm: filter \"envoy.filters.http.ext_proc\" called SendLocalReply after encode-side started; ignoring"`). Fixture-0023 scenario (c) re-routed from response_body `ImmediateResponse` → request_body `ImmediateResponse` (decode side is the well-supported path; SendLocalReply on decode side is the standard rejection mechanism). Closure deferred to a future phase (likely an HCM-side amendment to the late-arrival path during encode chain execution).
- **(III) Decode-side body-mutation-delivery limitation** (per ADR-0168 §Consequences refresh at IMPL Task 7 — Option B carry-forward) — body bytes captured by HCM BEFORE the chain mutation lands at the upstream wire-write path; the processor SEES the body envelope + can mutate `decodeBodyBuf`; Content-Length reconciles via the shared headers map; the upstream `echobackend` sees the ORIGINAL request body bytes verbatim (NOT the mutated body). Fixture-0023 scenario (a) issues a non-empty body solely for the body-stage outbound dispatch exercise + the D5 attribute-roster scrape; the substantive decode-side delivery story closes in a future phase. The 19.2 IMPL holds the decode-side mutation contract at the in-process `decodeBodyBuf` layer (unit-tested at IMPL Task 6); the upstream-delivery extension surfaces as a future-phase HCM amendment.

**Race-discipline pins (per ADR-0171 §Consequences refresh at IMPL Task 4 + race-test landings at IMPL Task 8):** D9-discipline (NO per-stream mutex on the framework's sequential decode→encode dispatch invariant + the bidi-stream single-in-flight-message correlation rule) RATIFIED-AT-IMPL-TIME via the Task 8 race-test surface — `TestOnDestroyDuringBodyStageOutbound`, `TestEncodeBufConcurrency_*`, `TestPerMessageTimer_*`, `TestModeOverrideVsBodyStageDispatch`. The race detector observes ZERO data-race violations across the 4-stage body-mode dispatch + the encode-side body-buffering chain extension + the per-message timer cancel/rebuild surface.

**`response_code_details` joint divergence-window (phases 16 + 17 + 18.1 + 18.2 + 19.1 + 19.2):** UNCHANGED by 19.2 — the body-stage `ImmediateResponse` deny path also does NOT emit `response_code_details` (envoy-go MVP joint divergence with phases 16/17/18). Reference Envoy v1.37.2 emits `ext_proc_*` family details strings on the deny path. Five §9 filters (rbac, jwt_authn, ext_authz-HTTP, ext_authz-gRPC, ext_proc — the latter now spanning both 19.1 + 19.2) contribute to this joint deferred cluster.

**Dynamic-metadata family joint divergence-window (phases 16 + 17 + 18 + 19.1 + 19.2):** UNCHANGED by 19.2 — body-mode mechanics are orthogonal to dynamic-metadata; no new dynamic-metadata fields activate at 19.2. ext_proc continues as the FOURTH §9 filter blocked on the dynamic-metadata family.

**No new ADR-0044 escape-valve fired at 19.2 IMPL (D10 hypothesis HELD):** the planner-time D10 hypothesis predicted NO impl-time-unanticipated ADR fires at 19.2 IMPL (the SPEC-time anticipated surfaces — buffered-body-release-vs-stream-reset interaction + mutation_rules application to body bytes — settled within the existing ADR envelope). The IMPL confirms: 4 §Decision-touchpoints landed at their per-Task Lands-in-Tasks (ADR-0175 §Decision + §Consequences full body at IMPL Task 2; ADR-0168 §Decision AMENDMENT in-place at Task 3 + §Consequences refresh at Task 7; ADR-0171 §Decision AMENDMENT in-place at Task 4; ADR-0172 §Decision AMENDMENT in-place at Task 6); ADR-0177 stays unconsumed (reserved for future-phase surfaces). The 19.2 IMPL closed the per-message-timer carry-forward + the D5 attribute-roster crystallization + the body-mode PARSE-REJECT lift at the 4 §Decision-touchpoints; the two empirical-pin AMENDMENTS surfaced at the fixture-0023 scrape are SCOPE-REDUCTIONS of the scenario surface, NOT new ADR-grade decisions.

### Phase 20 forward-pointer notes

**Phase 20 closes 2 prior-phase forward-pointers** at phase-20 phase-done per phase-20 SPEC §9 item A:

- **ADR-0159 §Future Work forward-pointer** ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction"): **CLOSED at phase 20** per phase-20 SPEC §3.5 IN-PLACE ADR-0159 §Decision AMENDMENT + §Future Work CLOSURE-AT-PHASE-20 paragraph. **FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer.** The third-consumer trigger condition fired exactly as ADR-0159 anticipated: jwks Fetcher (phase 17) + extauthz httpAuthClient (phase 18.1) + oauth2 token_endpoint POST (phase 20 NEW) = 3 consumers → extraction trigger → `internal/httpclient/` framework primitive (per ADR-0177). See `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159) → CLOSURE-AT-PHASE-20` paragraph above.
- **ADR-0150 implicit forward-pointer** (jwks Fetcher cross-phase consumer of future httpclient primitive): **CLOSED at phase 20** per phase-20 SPEC §3.4 IN-PLACE ADR-0150 §Decision AMENDMENT. Minor closure (no load-bearing protocol decision was awaiting it). See `## JWKS framework primitive (per phase 17 ADR-0150) → REFACTORED-AT-PHASE-20` paragraph above.

**Deferred items — 17 deferrals + 2 NEVER-DEFERRED permanent absences (per phase-20 SPEC §8 — 12 OAuth2Config field deferrals + 3 SDS/proto-shape deferrals + 2 permanent absences):**

- **`OAuth2Credentials.basic_auth` (BASIC_AUTH `client_secret_basic`)** — PARSE-REJECT permanently at MVP per phase-20 SPEC §2.3 + AMEND-5. envoy-go uses `client_secret_post` exclusively (the 4-field auth-code template embeds `client_secret` in the POST body).
- **`OAuth2Config.retry_policy`** — DEFERRED per phase-20 SPEC §20.P1 RATIFIED + §2.10. MVP `internal/httpclient/` applies the zero-retry default (matches upstream wire behavior at the token_endpoint POST). Re-activation: a future operator-ergonomics phase wires `RetryPolicy` through the existing `Options` envelope without a Client signature break.
- **id_token-and-jwks-and-jwt-verifier NEW deferred-cluster (anchored at phase 20)** — 1-deep at phase 20; resurfaces at the future id_token-enabling phase. Bundles `OAuth2Config.id_token` (per §2.2) + the authorization-server JWKS round-trip + the `OAuth2Config.end_session_endpoint` (per §2.4 — RP-initiated logout per OIDC) + the IdToken cookie envelope position + the `oauth_id_token` CookieName + `disable_id_token_set_cookie`. Phase-20 IMPL does NOT add a NEW jwks consumer (the ADR-0150 in-place refactor refactors the EXISTING jwt_authn consumer only; ADR-0151 jwt verifier NOT consumed).
- **PKCE envelope** (`use_pkce` + `oauth_nonce` + `code_verifier` + `code_verifier_token_expires_in`) — PARSE-REJECT permanently at MVP per phase-20 SPEC §2.1 + AMEND-5. MVP emits the 4-field auth-code template; the PKCE-gated 5th `code_verifier` field stays absent until the PKCE-enabling phase lands per AMEND-5 + ADR-0185 §Decision.
- **`OAuth2Config.cookie_configs`** (`*CookieConfigs` wrapper per AMEND-6 C2) — DEFERRED per phase-20 SPEC §2.5. MVP uses listener-default Set-Cookie attributes (`Path=/; Secure; HttpOnly; SameSite=Lax`). The **`Partitioned` cookie attribute (CHIPS-style)** also DEFERRED per AMEND-7 + §8 item 15 (NEW deferral surface NOT anticipated by BRAINSTORM; depends on `cookie_configs` activation).
- **`OAuth2Config.{disable_access_token_set_cookie, disable_refresh_token_set_cookie}`** — DEFERRED per phase-20 SPEC §2.6. MVP always emits BearerToken + RefreshToken cookies (the latter when `use_refresh_token=true`).
- **`OAuth2Config.csrf_token_expires_in` explicit field-consumption** — DEFERRED per phase-20 SPEC §20.P12 RATIFIED + §2.7. MVP uses proto-default 600s (10 minutes) via proto-default fall-through.
- **`OAuth2Credentials.cookie_domain`** (per AMEND-6 C1; field is on `OAuth2Credentials` field 5, NOT on `OAuth2Config` as BRAINSTORM §1.1 stated) — DEFERRED per phase-20 SPEC §20.P2 RATIFIED + §2.9. MVP emits host-only cookies (no `Domain=` attribute). **The empty default carries the host-only cookie invariant** load-bearing for the HMAC `domain` empty-string subtlety (recorded at the `### envoy.filters.http.oauth2 → #### Cookie envelope discipline` subsection above — the callback emit-site uses `domain=""` as the HMAC input because the upstream-bound redirect carries no authority context to anchor against; subsequent validation-site requests produce the same `domain=""` HMAC input ONLY when cookies are host-scoped; when a future cookie-domain-enabling phase lifts the empty default, the validation site's non-empty `domain` will NOT match the emit-site's `domain=""` HMAC unless the future phase threads the inbound request authority through to the callback emit-site OR inlines the HMAC `domain` input as the redirect_uri's host parsed once at parse-time).
- **SDS non-filesystem ConfigSource variants** (`ApiConfigSource` + `Ads` oneof arms; deprecated `path` field 1) — DEFERRED + PARSE-REJECT per phase-20 SPEC §3.2 + §20.P6 RATIFIED + §2.11. **NEW deferred-cluster anchored at phase 20** (SDS-non-filesystem). Re-activation: the future xDS-CDS / xDS-SDS-enabling phase lands the non-filesystem ConfigSource variants alongside `internal/sdsfile/` peer packages.
- **`generic_secret.secret_file` arm** (filesystem alternative to `inline_string`) — DEFERRED + PARSE-REJECT at MVP per phase-20 SPEC §8 item 14. The framework watches the outer Secret-proto JSON/YAML file via fsnotify; the inner double-indirect loading is not modeled at MVP.

**Permanent absences (2 — NOT deferrals):**

- **Per-route override** — the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all per phase-20 SPEC §5.1 + §20.P7 RATIFIED. Listener-scoped only; HCM-parse-time PARSE-REJECT for any route-level `typed_per_filter_config` entry per §5.2. **THIRD CONSECUTIVE §9 family-row to make this REUSE-by-absence decision** (after phase 18 + phase 19 — strictly stronger than the prior two phases' 5th-canonical REUSE). Permanent absence; ADR-0125 roster NOT extended (see `## Per-route canonical patterns cross-reference` below for the phase-20 cross-reference paragraph).
- **Runtime feature gate** — envoy-go has no runtime-features layer per phase-20 BRAINSTORM S2 settled. Upstream's `envoy.reloadable_features.oauth2_encrypt_tokens` reloadable-features gate NOT modeled. MVP relies on the `disable_token_encryption` proto-field default (false) as the sole switch.

**`response_code_details` joint divergence-window (phases 16 + 17 + 18.1 + 18.2 + 19.1 + 19.2 + 20):** EXTENDED by phase 20 per phase-20 SPEC §9 item 4 — oauth2's 401 deny-path candidate for `response_code_details` but upstream Envoy v1.37.2 also does not emit one (no candidate-emission analog upstream); phase 20 ADDS to the joint-closure forward-pointer. **Six §9 filters** (rbac, jwt_authn, ext_authz-HTTP, ext_authz-gRPC, ext_proc, oauth2) now contribute to this joint deferred cluster — strengthens the case for a dedicated `response_code_details` framework phase.

**Dynamic-metadata family joint divergence-window (phases 16 + 17 + 18 + 19.1 + 19.2):** UNCHANGED by phase 20 — oauth2 has NO metadata-emit surface (the OAuth 2.0 filter's primary contract is the 5-cookie envelope + the 6-counter stat surface; no dynamic-metadata is produced or consumed at MVP). Cluster STAYS at 5 §9 filters blocked per phase-20 SPEC §9 item 3. Phase 20 is the FIRST §9 row in five consecutive phases (16, 17, 18, 19.1, 19.2) to NOT extend this divergence-window.

**HMAC `domain` empty-string subtlety (discovered at phase-20 IMPL Task 12 follow-up at commit `6396eab`):** the callback emit-site at `emitCategoryB_PostCallbackLocked` uses `domain=""` as the HMAC input because the upstream-bound redirect carries no authority context to anchor against. This is documented load-bearing in two places: (1) the `### envoy.filters.http.oauth2 → #### Cookie envelope discipline` subsection above (the substantive contract), and (2) the `OAuth2Credentials.cookie_domain` DEFERRED bullet above (the future-phase alignment concern). The MVP default config (empty `cookie_domain`, host-only cookies per §20.P2) is the only validated path; a future cookie-domain-enabling phase MUST either (1) thread the inbound request authority through to the callback emit-site (currently lost because `handleCallback` doesn't preserve the host across the async-resume boundary), or (2) inline the HMAC `domain` input as the redirect_uri's host parsed once at parse-time.

**Fixture-0024 cross-side promotion forward-pointer (deferred at phase-20 IMPL Task 12 DONE_WITH_CONCERNS):** the differential fixture `0024-http-oauth2` (per phase-20 SPEC §7) ships at phase-20 phase-done as a **reference-less wire-shape oracle** (envoy-go-side execution against in-process `test/helpers/oauthbackend/`; expectations.yaml captures the envoy-go wire shape per scenario). True cross-side differential against reference Envoy v1.37.2 is DEFERRED because: (1) the `disable_token_encryption=true` scenario (i) per SPEC §7.1 row 9 requires the v1.32.4 → v1.37.x `go-control-plane` bump to land oauth2 proto support; (2) the AES-CBC random-IV + wall-clock-driven OauthExpires non-determinism on the byte-comparison axis requires designing an accept-list of non-deterministic fields OR injection hooks for deterministic IV / timestamp generation under fixture context. Both surfaces close at the future `go-control-plane v1.37.x` bump phase + the future fixture-determinism-hooks phase respectively. The structural wire-up at IMPL Task 12 follow-up (commit `6396eab` — `handleCallback` auth-code POST + `applyTokenEndpointResponse` full disposition matrix + 9 new callback unit tests) is the critical fix that unblocks the auth-code flow end-to-end without the cross-side promotion.

**v1.32.4 → v1.37.x go-control-plane bump forward-pointer (deferred):** the current `github.com/envoyproxy/go-control-plane` version is v1.32.4 (per phase-20 BRAINSTORM cold-start verification + the existing repo go.mod pin). The oauth2 proto support landed at v1.37.x; phase-20 IMPL ships against a pinned subset of the proto package reachable from v1.32.4 via the in-tree generated bindings. The next-phase bump is anticipated when (a) a §9 filter requires a v1.37.x-introduced proto surface NOT present at v1.32.4, OR (b) the fixture-0024 cross-side promotion above schedules. The bump is anticipated as a stand-alone framework-cleanup task; it does NOT block phase-20 phase-done.

**Auth-code POST wire-up gap RESOLVED-AT-TASK-12-FOLLOWUP (commit `6396eab`):** the SKELETON `handleCallback` shape at phase-20 IMPL Task 5 (state-cookie validation only; no outbound POST initiation) was a known SKELETON gap acknowledged in the Task 5 docstring + tracked as Task-12 future-work. Task 12 surfaced the gap at fixture-0024 scenario (a) — the `sign_in_happy_path/` scenario could not exercise the full sign-in flow end-to-end because the auth-code leg never initiated the token_endpoint POST. **CLOSED at Task 12 follow-up commit `6396eab`** — `handleCallback` now initiates the async outbound POST via `cc.tokenEndpointPoster`; `applyTokenEndpointResponse` implements the full §4.5 + §4.7 + AMEND-3 disposition matrix (2xx success → category-(b) 302; 5xx retry-eligible → category-(a) 302; 4xx terminal → category-(d) 401; transport-error / malformed-JSON / nil-poster fail-safe → category-(a) 302). 9 new tests added covering the full disposition matrix; the pre-existing Task 5 SKELETON test renamed to `_Skeleton` suffix with its trivial-path assertion retained. NO new ADR consumed (the disposition was already specified at SPEC §4.5 + §4.7 + AMEND-3 + ADR-0180 + ADR-0183 — the closure was a structural SKELETON-to-full-body shift, not a new design surface). The HMAC `domain` empty-string subtlety surfaced at this follow-up's review (recorded above as a forward-pointer load-bearing for the cookie-domain-enabling phase).

**No new ADR-0044 escape-valve fired at phase-20 IMPL (D11 hypothesis HELD):** the planner-time D11 hypothesis predicted NO impl-time-unanticipated ADR fires at phase-20 IMPL (the SPEC-time anticipated surfaces — AES-256-CBC PKCS#7 padding-oracle hardening + fsnotify event-debounce edge-cases + urlEncode charset edge-cases for non-ASCII bytes — settled within the existing ADR envelope per SPEC §10 item C ADR-0044 escape-valve reserve). The IMPL confirms: 9 NEW ADRs (ADR-0177..ADR-0185) §Decision + §Consequences bodies landed at their per-Task Lands-in-Tasks (ADR-0177 + ADR-0150 + ADR-0159 at Task 2; ADR-0178 at Task 3; ADR-0179 at Task 4; ADR-0180 at Task 5; ADR-0181 at Task 6; ADR-0182 at Task 7; ADR-0183 at Task 8; ADR-0184 at Task 9; ADR-0185 at Task 10) + 2 IN-PLACE §Decision AMENDMENT bodies landed at Task 2 (ADR-0150 jwks Fetcher refactor + ADR-0159 extauthz httpAuthClient refactor + ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph); ADR-0186 stays unconsumed (reserved for future-phase surfaces).

### Phase 21 forward-pointer notes

**Phase 21 closes 0 prior-phase forward-pointers** at phase-21 phase-done (no prior-phase load-bearing forward-pointer was awaiting phase 21).

**Deferred items — 8 deferrals (per phase-21 SPEC §8):**

- **RTDS runtime keying** (per phase-21 SPEC §8 item 1) — DEFERRED + PARSE-REJECT per phase-21 SPEC §5.2 + ADR-0187. MVP honors only `enabled.default_enabled` (static-default OFF); `enabled.runtime_key != ""` triggers HCM-parse-time PARSE-REJECT. Re-activation: the future Runtime/RTDS family phase lands the runtime consultation path + lifts the PARSE-REJECT arm per ADR-0187 §Future Work.

- **Cross-side byte-exact algorithmic parity** (per phase-21 SPEC §8 item 2) — DEFERRED. Sorted-slice quantile divergence vs upstream CircllHist (~1 bin-width per the envoy-go-strict departure above) means gradient + new-limit + sampled-RTT values are not cross-side byte-exact. Re-activation: a future cross-side fixture-0025-extension phase swaps to CircllHist OR accepts the divergence with a documented tolerance band.

- **Alternative ConcurrencyControllerConfig oneof arms** (per phase-21 SPEC §8 item 3) — DEFERRED. Phase 21 supports only the `GradientControllerConfig` oneof arm (the only arm at upstream v1.37.x). If upstream adds a non-Gradient-1 alternative (e.g., a fixed-window controller), phase 21's PARSE-REJECT for `concurrency_controller_config` other-than-Gradient enforces the gap until the alternative-controller phase lands.

- **CircllHist percentile-aggregation upgrade** (per phase-21 SPEC §8 item 4) — DEFERRED. See "sorted-slice-vs-CircllHist" departure record above. Re-activation: a future algorithmic-fidelity-extension phase introduces the CircllHist primitive in `internal/stats/circllhist/` and swaps the `Quantile` helper.

- **`fixed_value` static-minRTT alternative path** (per phase-21 SPEC §8 item 5 + AMEND-1 C4) — DEFERRED + PARSE-REJECT per phase-21 SPEC §5.3 + ADR-0186 §Consequences (d). MVP requires `min_rtt_calc_params.interval`. **Note on v1.32.4 proto-binding limitation (discovered at phase-21 Task 2 IMPL):** the `fixed_value` field is NOT exposed in the go-control-plane v1.32.4 Go binding (only v1.37.0+); the byte-stable PARSE-REJECT wording for this arm is preserved but the arm is structurally unreachable until a proto-bump. Re-activation: future proto-bump phase exposes the field + the static-path controller-state-machine extension phase lands the alternative path (no dynamic recalc; no 5-consec-trigger; no jitter).

- **Multi-listener controller-state-isolation explicit verification** (per phase-21 SPEC §8 item 6) — DEFERRED. Phase 21's fixture-0025 uses a 3-listener subject topology but does NOT explicitly assert that the per-listener `*gradientController` instances are state-isolated (e.g., scenario b's `concurrencyLimit` mutations don't leak to scenario a's stats). Re-activation: a future fixture-extension phase adds a 2-listener concurrent-load scenario explicitly asserting per-listener controller-state isolation.

- **`min_rtt_calculation_active` Accumulate import-mode parity** (per phase-21 SPEC §8 item 7) — DEFERRED. Phase 21 emits `min_rtt_calculation_active` as 0/1 (this filter's view); upstream Envoy v1.37.2 may use the Accumulate import-mode (cluster-aggregating across HCM instances) in some deployments. Re-activation: a future stats-import-mode framework phase introduces the Accumulate primitive + lifts this divergence.

- **`response_code_details` "reached_concurrency_limit" emission** (per phase-21 SPEC §8 item 8 + SPEC §12 item A3) — DEFERRED. envoy-go MVP has no access-log surface — the response_code_details field is not emitted; the 503 deny path's pseudo-`response_code_details` lives only in code comments. Re-activation: when the access-log framework phase lands, the 503 deny path can register the `reached_concurrency_limit` slot. **`response_code_details` joint divergence-window EXTENDED by phase 21 — seven §9 filters** (rbac, jwt_authn, ext_authz-HTTP, ext_authz-gRPC, ext_proc, oauth2, adaptive_concurrency) now contribute to this joint deferred cluster.

**No new ADR-0044 escape-valve fired at phase-21 IMPL (D8 hypothesis HELD):** the planner-time D8 hypothesis predicted NO impl-time-unanticipated ADR fires at phase-21 IMPL (the SPEC-time anticipated surfaces — sorted-slice quantile edge-cases + fakeClock determinism + ADR-0059 §Decision AMENDMENT scope-creep — all settled within the existing ADR envelope per SPEC §10 item C ADR-0044 escape-valve reserve). The IMPL confirms: 2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed at per-Task Lands-in-Tasks (ADR-0186 at Task 3; ADR-0187 at Task 2) + 1 IN-PLACE §Decision AMENDMENT body at Task 4 (ADR-0059 — REPLACES the SPEC-commit ANTICIPATION paragraph per ADR-0044 in-place edit discipline). ADR-0188 + ADR-0189 stay UNCONSUMED (STRENGTHENED two-slot escape-valve buffer per SPEC §10 D).

**Fixture-0025 cross-side promotion forward-pointer (deferred at phase-21 IMPL Task 10 DONE_WITH_DEVIATION):** the differential fixture `0025-http-adaptive-concurrency` (per phase-21 SPEC §7) ships at phase-21 phase-done as a **reference-less wire-shape oracle** (envoy-go-side execution against the existing HTTPSlowStream subprocess backend; per-scenario expectations captured at scenario assertion log). True cross-side byte-exact differential against reference Envoy v1.37.2 on scenario (b) overflow_503 is DEFERRED — RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION. The 3-listener subject topology + the byte-pinned 503 wire shape + the `rq_blocked` counter increment are all asserted envoy-go-side; the cross-side `CompareBytes` against v1.32.4-pinned reference Envoy needs (a) reference container support + (b) deterministic 2-concurrent-request ordering across both proxies. Both surfaces close at a future cross-side fixture-extension phase.

### Phase 22.1 forward-pointer notes

**Phase 22.1 closes 0 prior-phase forward-pointers** at phase-22.1 phase-done (no prior-phase load-bearing forward-pointer was awaiting phase 22.1). Phase 22.1 self-closes a substantial in-phase RATIFIED-PENDING-IMPL list: §13-R1 (`BootRejectFixture` driver interface — landed at Task 13 + Task 15); §13-W (envoy-go-side `"script load error: "` wording-pinning — landed at Task 15); §11.7.5 (substring assertion firing on both sides — verified at Task 15 fixture-0026 scenario (g) GREEN); §11.7.7 D3 closure (scenario (e) stat-counter `executions` delta IS the "Lua ran" assertion per option (a) — locked at PLAN session, verified at Task 14 driver `AssertStats`); §12-D1 (REFUTED — arms 5 + 17 silent no-op per Task 2 upstream-scrape evidence); §13-R6 D-P10 R6 (STANDS WEAK-default — `ns/op = 69865` ~70µs/stream << 1ms threshold at Task 12; ADR-0190 NOT consumed); §13-R3 (envoy-go headers-map is unordered `net/http.Header` + bridge `__pairs` alphabetical-snapshot RATIFIED at SPEC + landed at Task 6); D5 (28th-fuzzer count CONFIRMED at 28 at Task 11 — fuzzer `FuzzLuaConfigParse` lands clean).

**Deferred items — 22.2 BRAINSTORM scope hand-off:**

- **Full bridge surface delta** (per parent §10 + phase-22.1 SPEC §1) — body methods (`:body()`, `:bodyChunks()`, `:trailers()`); metadata (`:metadata()`, `:dynamicMetadata()`); `:httpCall()` (consumer #1 of `internal/httpclient/` per phase-20 ADR-0177); crypto helpers (`:base64Decode()`, `:sha256()`); filesystem helpers (`:fileBytes()`); `:timestamp()`; `:connection()`; full `:streamInfo()` (route-metadata / cluster-info / SSL-context / dynamic-metadata accessors). All defer to 22.2 IMPL. Anticipated ADRs (settled at 22.2 BRAINSTORM): ~2-4 NEW ADRs (full-bridge-API shape + httpCall dispatcher + body-buffering interaction with ADR-0128 + dynamic-metadata-bridge deferral). Likely +2 httpCall counters extend the stat surface (settled at 22.2 SPEC; project total would advance 102 → ~104 at 22.2 phase-done).
- **`*LState`-pool design (ADR-0190 escape-valve)** — D-P10 R6 STANDS WEAK-default at 22.1 IMPL (`ns/op = 69865` ~70µs/stream << 1ms threshold per parent §13-R6); ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. 22.2 may re-evaluate against the body/trailer bridge surface (which adds more bridge methods + more per-stream allocation); if the per-stream construction cost crosses the 1ms threshold there, ADR-0190 fires at 22.2 IMPL with the `*LState`-pool design.
- **AMEND-9 gopher-lua-vs-LuaJIT divergence catalogue extension** — the (a) `tostring(float)` Go shortest-round-trippable vs LuaJIT `"%.14g"`, (b) `string.format("%d", float)` Go-fmt-mismatch, (c) `pcall` error-message prefix divergences are forward-pointed by AMEND-9 for 22.2 (`lua.FormatNumber(v) string` helper recommended on `internal/lua/` for 22.2 use). 22.1's headers-bridge scope intentionally avoids these surfaces; 22.2's body/trailers bridge cannot.

**Deferred items — 22.3 BRAINSTORM scope hand-off:**

- **`Lua.SourceCodes` multi-script map activation** — arm 4 PARSE-REJECT lifts at 22.3; multi-script lookup via the `SourceCodes` map enables per-route delegation to named scripts.
- **`LuaPerRoute` 3-arm oneof override** — arm 18 PARSE-REJECT lifts at 22.3; NEW 9th canonical per-route shape per ADR-0125 §(xiv) AMENDMENT body landing at 22.3 IMPL final Task (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`). The §(xiv) AMENDMENT-anticipation paragraph already anchored at parent SPEC commit; AMENDMENT body lands at 22.3 IMPL final Task per ADR-0044 in-place edit discipline. ADR-0125 roster grows 8 → 9 at 22.3 phase-done — ENDS the FOUR-CONSECUTIVE ADR-0125-skip streak (phases 18+19+20+21 all skipped per the phase-21 ROADMAP cell).
- **Per-route 3-tier dispatch** — listener-default → SourceCodes-named-script → per-route DataSource override; settled at 22.3 SPEC.

**Deferred items — fuzzer-surfaced future-hardening pointers:**

- **`/dev/full`-class infinite-read OOM-kill defense** — Task 11 fuzzer surfaced arm 9-extension (16 MiB cap on `Filename` DataSource via `io.LimitReader`). Pattern applies broadly to any future file-reading DataSource consumer in the codebase; the local fix at `internal/filter/http/lua/datasource.go::resolveDataSourceFilename` is the first instance. A future bootstrap-hardening phase could lift the cap discipline to a shared `internal/safefile/` primitive. Documented for future-maintainer awareness; out of scope at 22.1.
- **`stats.IsValidName` pre-check at every operator-supplied stat-prefix consumer** — Task 11 fuzzer surfaced arm 19 at the lua filter's `Lua.stat_prefix` consumer. The pattern already exists at `hcm/config.go:209` + `cluster/manager.go:205`; the lua filter inherits the pattern at this 22.1 IMPL. A future cross-cutting audit could verify every operator-supplied stat-prefix consumer in the codebase has the pre-check (the boot-time-panic discipline per ADR-0059 is fail-loud but operator-hostile; pre-check + PARSE-REJECT is operator-friendly). Out of scope at 22.1.

**No new ADR-0044 escape-valve fired at phase-22.1 IMPL (D-P10 hypothesis HELD):** the planner-time D-P10 R6 escape-valve gate (per parent §13-R6 RATIFIED-PENDING-IMPL) evaluated GREEN — `ns/op = 69865 ≤ 1_000_000` threshold; ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. 2 NEW ADR §Context drafts + §Decision + §Consequences bodies landed at the 22.1 IMPL Task 16 atomic landing (ADR-0188 NEW `internal/lua/` framework primitive + ADR-0189 NEW `internal/filter/http/lua/` package shape; §Context anchored at parent SPEC commit per parent SPEC §4.1 + §4.4 + ADR-0044 in-place edit discipline). ADR-0125 §(xiv) AMENDMENT-anticipation paragraph UNCHANGED at this 22.1 IMPL (anchored at parent SPEC commit; AMENDMENT body lands at 22.3 IMPL final Task). The 22.1 PROGRESS Task 11 also documents 2 PARSE-REJECT arms surfaced at fuzzing time (arm 19 stat_prefix-invalid + arm 9-extension filename-too-large; roster 18 → 19) — both fixed inline at Task 11 per ADR-0018 fuzzer-discipline ("fuzzers exist to surface panics + the panics must be fixed before the fuzzer lands").

## Per-route canonical patterns cross-reference (ADR-0125 roster; updated through phase 24 — `RateLimitPerRoute` 10th-canonical AMENDMENT 9 → 10 LANDED at 24.2 IMPL Task 3 per ADR-0125 §(xv) + ADR-0199)

*This section summarizes the ADR-0125 per-route canonical pattern roster as of phase 19.2. ADR-0125 governs `typed_per_filter_config` resolution; each canonical was introduced or extended at a specific phase. Phase 19.2 does NOT modify the roster (no ADR-0125 amendment fires per phase-19.2 planner-time D12) — `ExtProcOverrides.processing_mode` body-mode arms become CONSUMED for the gRPC-service arm at 19.2 per the existing 5th-canonical REUSE, but the canonical-pattern roster itself stays at 8 entries.*

| Canonical | Shape | Introduced | Consumers (as of 19.1) |
|---|---|---|---|
| §(i) 1st — listener-fallback | No per-route key at all → listener-level config applies | phase 07.1 | cors, fault, header_mutation, local_ratelimit, csrf, buffer, compressor, bandwidth_limit, rbac, jwt_authn, ext_authz |
| §(ii)–§(iv) 2nd–4th | (See ADR-0125 §(ii)–§(iv) for superseded/legacy patterns) | — | — |
| §(v) 5th — disabled-OR-NARROWER-override | `oneof{disabled(bool) | narrower_config(Message)}` in a PGV-required oneof | phase 13 buffer | buffer (phase 13), compressor (phase 14), **ext_authz (phase 18.1 — FIRST §9 REUSE; NO new amendment)**, **ext_proc (phase 19.1 — SECOND CONSECUTIVE §9 REUSE; NO new amendment; ADR-0173 records the explicit no-amendment classification)** |
| §(vi) 6th — bare-message-via-TPFC | Same `BandwidthLimit` proto at per-route level; code-level-required `limit_kbps` | phase 15 | bandwidth_limit (phase 15) |
| §(vii) 7th — absent-implies-disabled | No per-route key → filter active; per-route key present → filter disabled (inverse of 1st) | phase 16 | rbac (phase 16) |
| §(viii) 8th — string-reference-delegation | `oneof{disabled(bool) | requirement_name(string)}` → per-route does NOT carry filter config, references-by-name into listener-level map | phase 17 | jwt_authn (phase 17) |
| §(xiv) 9th — 3-arm-hybrid (disabled-bool + string-reference + DataSource-wholesale-override) | `oneof{disabled(bool) | name(string→listener-SourceCodes) | source_code(DataSource)}` | phase 22.3 | lua (phase 22.3) |
| §(xv) 10th — data-only-with-vh-inclusion-enum | `RateLimitPerRoute{vh_rate_limits(enum OVERRIDE/INCLUDE/IGNORE) + rate_limits[]([]RateLimit Axis-A) + override_option(enum accepted-but-INERT) + domain(string descriptor-tier override)}` | phase 24.2 | ratelimit (phase 24.2) |

**Phase 18.1 ext_authz cross-reference:** `ExtAuthzPerRoute.oneof override` with `disabled` arm (PGV `const: true` — PARSE-REJECTS `disabled: false`) + `check_settings` arm (narrower `CheckSettings` sub-message). Maps cleanly onto the **existing 5th canonical** (ADR-0125 §(v)). **NO §(xiv) amendment paragraph** — the FIRST §9 row to REUSE the 5th canonical since its introduction at phase 13. ADR-0163 records the explicit no-amendment 5th-canonical-REUSE classification.

**Phase 19.1 ext_proc cross-reference:** `ExtProcPerRoute.oneof override` with `disabled` arm (PGV `const: true` — PARSE-REJECTS `disabled: false`) + `overrides` arm (narrower `ExtProcOverrides` sub-message; MVP-CONSUMED `processing_mode` + `grpc_service`; 5 silent-ignored fields per ADR-0173). Maps cleanly onto the **existing 5th canonical** (ADR-0125 §(v)). **NO §(xiv) amendment paragraph** — the SECOND CONSECUTIVE §9 row (after phase 18.1 ext_authz) to REUSE the 5th canonical rather than extend the roster, strengthening the ADR-0125 roster-not-monotonic lesson. ADR-0173 records the explicit no-amendment 5th-canonical-REUSE classification (the absence of a §(xiv) amendment is itself a recorded decision). The 5th canonical now has FOUR consumers: buffer (phase 13) + compressor (phase 14) + ext_authz (phase 18.1) + ext_proc (phase 19.1). ADR-0125's canonical-pattern roster STAYS at 8 entries after phase 19.1.

**Phase 20 oauth2 cross-reference — REUSE-by-absence (STRONGER form):** the v1.37.x oauth2 proto has **NO `OAuth2PerRoute` message at all** per phase-20 SPEC §5.1 + §20.P7 RATIFIED (strongest-form evidence — the proto file has no per-route-override message arm). Listener-scoped only; HCM-parse-time PARSE-REJECT for any oauth2 `typed_per_filter_config` entry at route or virtualHost level per phase-20 SPEC §5.2 (consistent with the other listener-scoped filter PARSE-REJECT messages; `RegisterPerRouteValidator` factory method is the registration hook). **NO §(xv) amendment paragraph** — the **THIRD CONSECUTIVE §9 family-row** (after phase 18.1 ext_authz + phase 19.1 ext_proc) to NOT extend the ADR-0125 roster. Phase 20's REUSE-by-absence is a **STRONGER form of the lesson** than the prior two phases' 5th-canonical REUSE — there is no per-route surface at all, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT discipline rather than a roster-REUSE classification. The ADR-0125 roster does NOT grow monotonically; phase 20 strengthens the lesson WITHOUT amendment (the absence itself is the lesson). ADR-0180 records the explicit no-amendment classification (the absence of a §(xv) amendment is itself a recorded decision). ADR-0125's canonical-pattern roster STAYS at 8 entries after phase 20 (the 5th canonical's consumer roster ALSO STAYS unchanged at FOUR consumers — buffer (phase 13) + compressor (phase 14) + ext_authz (phase 18.1) + ext_proc (phase 19.1) — oauth2 does NOT join because there is no proto surface to REUSE the 5th canonical against).

**Phase 21 (adaptive_concurrency) — FOURTH CONSECUTIVE §9 row to skip ADR-0125 roster extension** (after phase 18 + phase 19 + phase 20). The v1.32.4 / v1.37.x adaptive_concurrency proto has NO `AdaptiveConcurrencyPerRoute` message at all per phase-21 SPEC §5.4 + ADR-0186 §Consequences (per-route REUSE-by-absence). HCM-parse-time PARSE-REJECT for any route-level `typed_per_filter_config` entry happens at proto-deserialization via the existing HCM framework — no phase-21-specific PARSE-REJECT code. The absence-as-recurring-pattern note: STRENGTHENED across 4 consecutive §9 rows; the next §9 row that DOES extend the per-route surface (whichever family-row introduces a new per-route message) will end the run. ADR-0125's canonical-pattern roster STAYS at 8 entries after phase 21 (the 5th canonical's consumer roster ALSO STAYS unchanged at FOUR consumers — buffer (phase 13) + compressor (phase 14) + ext_authz (phase 18.1) + ext_proc (phase 19.1) — adaptive_concurrency does NOT join because there is no proto surface to REUSE the 5th canonical against; mirrors the phase-20 oauth2 REUSE-by-absence shape). ADR-0186 records the explicit no-amendment classification (the absence of a §(xvi) amendment is itself a recorded decision).

**Phase 22.1 (lua headers-bridge) — DEFERS roster extension; ENDS the four-consecutive-§9-row ADR-0125-skip streak at the 22.3 sub-phase IMPL final Task per ADR-0125 §(xiv) AMENDMENT-anticipation paragraph anchored at parent SPEC commit.** The v1.37.x `Lua` proto carries `LuaPerRoute` (3-arm oneof: `disabled` + `name` string-reference into `Lua.SourceCodes` map + `source_code` DataSource wholesale-override) — see parent SPEC §5.1. At 22.1 the entire per-route surface PARSE-REJECTs at boot-level `RegisterPerRouteValidator` per arm 18 of the 19-arm PARSE-REJECT roster (per ADR-0110 single-chokepoint) with byte-stable wording `"lua: per-route configuration is not yet supported (lands in phase 22.3)"`. At 22.3 (per BRAINSTORM Q7 + parent SPEC §4.5) the NEW 9th canonical per-route shape (3-arm hybrid: `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`) lands as ADR-0125 §(xiv) AMENDMENT body — roster grows 8 → 9 at 22.3 phase-done. The §(xiv) AMENDMENT-anticipation paragraph is already anchored at parent SPEC commit `41ccee7` per parent §4.5 + ADR-0044 in-place edit discipline + ADR-0125 §(xiv); body lands at 22.3 IMPL final Task. This 22.1 IMPL Task 16 atomic landing makes NO edit to ADR-0125's roster (which stays at 8 entries) but the cross-reference caption above is bumped from "updated through phase 21" → "updated through phase 22.1; 9th canonical AMENDMENT-anticipation paragraph anchored at parent SPEC commit per ADR-0125 §(xiv) — body lands at 22.3 IMPL final Task" to surface the in-flight AMENDMENT anchor. ADR-0189 §Decision body cross-references the §(xiv) anticipation paragraph. **FIFTH CONSECUTIVE §9 row to NOT extend the ADR-0125 roster at IMPL final Task** (phases 18+19+20+21+22.1) — the streak ENDS at the 22.3 IMPL final Task per the AMENDMENT body landing schedule.

**Phase 23 (admission_control) — FIRST ADR-0125-skip since phase-22's roster amendment** (8 → 9 at 22.3). The v1.37.2 `AdmissionControl` proto has **no `AdmissionControlPerRoute` message at all** per phase-23 SPEC §5.4 + ADR-0195 §Consequences (per-route REUSE-by-absence — the strongest form; mirrors phase-20 oauth2 + phase-21 adaptive_concurrency). HCM-parse-time PARSE-REJECT for any admission_control `typed_per_filter_config` entry at route or virtualHost level (per ADR-0110 single-chokepoint; consistent with the other listener-scoped filter PARSE-REJECT messages). **NO new amendment paragraph** — the **FIRST §9 family-row since phase-22's roster growth** to NOT extend the ADR-0125 roster. ADR-0125's canonical-pattern roster STAYS at **9 entries** after phase 23 (UNCHANGED from phase-22.3). ADR-0195 records the explicit no-amendment classification (the absence of a §(xv) amendment is itself a recorded decision). The REUSE-by-absence absence-as-recurring-pattern lesson CONTINUES: admission_control does NOT join any canonical; the 9th-canonical's consumer roster (lua at phase-22.3) is ALSO UNCHANGED.

**Phase 24.1 (ratelimit — CORE-decision-path slice) — DEFERS roster extension; ENDS the post-phase-22.3 ADR-0125-skip streak at the 24.2 sub-phase IMPL final Task per ADR-0125 §(xv) AMENDMENT-anticipation paragraph anchored at parent SPEC commit.** The v1.37.x `RateLimit` proto carries `RateLimitPerRoute` (a NEW canonical per parent SPEC §5.3: "data-only-with-vh-inclusion-enum" — `vh_rate_limits` inclusion enum OVERRIDE/INCLUDE/IGNORE honored at runtime + route-additional `rate_limits[]` Axis-A composition + `override_option` accepted-but-ignored INERT per AMEND-4 + `domain` override). At 24.1 the entire per-route surface is **STUBBED-but-not-PARSE-REJECTED-at-boot** — `RateLimitPerRoute` TPFC entries pass HCM-parse-time validation but the runtime arms (Axis-B `vh_rate_limits` traversal + `domain` override + route-additional `rate_limits[]` composition) are NOT-CONSUMED at 24.1; the filter only walks the matched Route's `rate_limits[]` (Axis-A subset) at 24.1 per the parent SPEC §16 split axis. At 24.2 (per parent SPEC §5.3) the NEW 10th-canonical per-route shape lands as ADR-0125 §(xv) AMENDMENT body — roster grows 9 → 10 at 24.2 phase-done. The §(xv) AMENDMENT-anticipation paragraph is anchored at parent SPEC commit per parent §5.3 + ADR-0044 in-place edit discipline + ADR-0125 §(xv); body lands at 24.2 IMPL final Task per the 22.1→22.3 anticipation→landing precedent. The 24.1 IMPL atomic landing makes NO edit to ADR-0125's roster (which stays at 9 entries) but the cross-reference caption above is bumped from "updated through phase 23" → "updated through phase 24.1; 10th canonical AMENDMENT-anticipation paragraph anchored at parent SPEC commit per ADR-0125 §(xv) — body lands at 24.2 IMPL" to surface the in-flight AMENDMENT anchor. ADR-0197[core] §Decision body cross-references the §(xv) anticipation paragraph (the 24.1-slice does not extend the canonical-pattern roster; the 24.2-slice will). **SECOND CONSECUTIVE §9 row to NOT extend the ADR-0125 roster at IMPL final Task** (phases 23 + 24.1) — the streak ENDS at the 24.2 IMPL final Task per the AMENDMENT body landing schedule (mirroring the phase-22.1→22.3 anticipation→landing pattern).

**Phase 24.2 (ratelimit — per-route + headers slice) — LANDS the §(xv) AMENDMENT 9 → 10 paragraph; ENDS the phase-23 + 24.1 REUSE-by-absence skip streak per ADR-0199 + ADR-0125 §(xv).** The `RateLimitPerRoute` shape lands as the **NEW 10th canonical** per ADR-0125 §(xv) — a `data-only-with-vh-inclusion-enum` shape that DIVERGES from all 9 prior canonicals: (1) the proto carries DATA only (not a disabled-bool, not a string reference, not a wholesale-override sub-message) — operators encode the per-route intent purely through field values; (2) the `vh_rate_limits` inclusion enum (`OVERRIDE` / `INCLUDE` / `IGNORE` / DEFAULT=`OVERRIDE`) is the load-bearing axis that controls Axis-B cross-tier composition at request time (NOT at boot; the `rate_limits[]` from the Route + the VirtualHost are both retained at HCM-parse-time per the DELTA-2 plumbing); (3) the `rate_limits[]` field carries route-additional Axis-A descriptor policies (when non-empty, the per-route walk is EARLY-RETURN — only those policies fire, the matched-Route's `rate_limits[]` + the VirtualHost's `rate_limits[]` are BOTH ignored); (4) the `override_option` enum is PARSE-ACCEPTED-but-IGNORED INERT per AMEND-4 (the upstream proto field is `[#not-implemented-hide:]`-tagged; envoy-go honors the upstream-parity decision — no PARSE-REJECT; NOT a departure); (5) the `domain` field is a descriptor-tier override (when set, the descriptor sent to the RLS gRPC call carries the per-route `domain`; the listener-level `domain` is overridden; the stat namespace is UNCHANGED per AMEND-1). **The 10th canonical does NOT fit any prior pattern** (1st = listener-fallback / 5th = disabled-OR-narrower-override / 6th = bare-message-via-TPFC / 7th = absent-implies-disabled / 8th = string-reference-delegation / 9th = 3-arm-hybrid); ADR-0125 roster grows 9 → 10 at 24.2 Task 3 — the AMENDMENT body landed at the parent SPEC commit per ADR-0125 §(xv) + ADR-0199 §Decision. **THIRD §9 row in the family to extend the ADR-0125 roster** (phase 13 buffer §(v) 5th + phase 22.3 lua §(xiv) 9th + phase 24.2 ratelimit §(xv) 10th — the §(vi)/§(vii)/§(viii) per-phase extensions at 15/16/17 also extended). Cross-references: ADR-0199 §Decision body anchors the 10th-canonical AMENDMENT paragraph; ADR-0197 in-place §Decision amendment (24.2 Task 5) anchors the X-RateLimit DRAFT_VERSION_03 slice; ADR-0125 §(xv) anticipation→landing paragraph at the parent SPEC commit. **§9 family at 24.2 phase-done:** 17 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23 / 24 — the parent row 24 flips `in-progress → done` at 24.2 phase-done per the 18/19/22 ROLLUP precedent); §9 family closes to **1 remaining row: `wasm`**.
