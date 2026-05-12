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

### 60-name table (introduced by phase 06.1; extended by phase 09; extended by phase 11; extended by phase 12; UNCHANGED in phase 13; extended by phase 14; extended by phase 15)

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

**Total: 64 internal names** (17 from 06.1 + 5 from 09 + 4 from 11 + 3 from 12 + 0 from 13 + 17 from 14 + 14 from 15 + 4 from 16). The four `downstream_rq_Nxx` and four `upstream_rq_Nxx` Prometheus exposition forms collapse to two base-name groups (one HCM, one cluster) per the Rule SN4 status-class flattening discipline. The 2 deferred histograms (phase 15) + the per-policy counter family (phase 16; operator-config-driven) are documented separately; they do NOT count in the 64-name base total.

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
