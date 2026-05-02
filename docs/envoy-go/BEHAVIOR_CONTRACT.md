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
| Stats output | Per-stat behavioral delta after defined load is equal between envoy-go and reference Envoy. Gauges are snapshot-equal after drain. Names + label keys + types byte-equal; HELP text ignored. Allow-list: 17 stats listed in § Stat-name mapping. All other Envoy stat names in /stats/prometheus output are ignored by the differential. |
| xDS wire behavior | ADS message sequences match the protocol state machine; effective-config diff on identical snapshots |
| Timing | Not compared by default; a phase may opt in to latency bounds |
| HTTP filter chain | Per-request equivalence on cors preflight + actual-request response shapes (status + header set + body) between envoy-go and reference Envoy. Filter iteration order, sendLocalReply encode-chain entry, and 413 overflow shape are verbatim-pinned at the ENVOY_TARGET SHA. Differential covers cors only; `envoy.filters.http.envoy_go_test` excluded (test-only); other filters in the §9 family are future-phase scope. |
| Listener filters | Per-connection chain-selection equivalence: which `filter_chain` is dispatched is byte-equal across envoy-go and reference Envoy. Verified via per-connection backend-port routing in fixture 0008. Chain-match precedence ordering, `default_filter_chain` fallback semantics, and empty-match-vs-default resolution are verbatim-pinned at the ENVOY_TARGET SHA. Differential covers chain-selection only (which backend each connection is routed to); listener-filter internal byte-level behavior (e.g., tls_inspector parser output) is unit-tested only. |
| Admin /config_dump | Body byte-equal modulo build/timestamp/uptime allow-list. Three-envelope ordering: Bootstrap, Listeners, Clusters. Allow-list: `bootstrap.node.user_agent_name`, `bootstrap.node.user_agent_build_version`, `bootstrap.node.extensions[]`, `<*ConfigDump>.last_updated` per-field allow-listed. dynamic_* arrays absent in both. (Per phase 08.1 SPEC §13.2.) |
| Admin /clusters | Tuple-set equality on `(cluster, key, value)` triples. envoy-go emits Envoy's full unconditional 28-line-per-cluster + 18-line-per-endpoint set with default constants for non-modeled fields. Allow-list: hot-path counters `cx_total`, `cx_connect_fail`, `rq_total`, `rq_active`, `rq_error` allow ±1 tolerance. (Per phase 08.1 SPEC §13.2.) |
| Admin /listeners | Body byte-equal (after framing dechunk). Single line per listener. No allow-list. (Per phase 08.1 SPEC §13.2.) |
| Admin /server_info | Body byte-equal modulo build/uptime/CLI-flags/node allow-list. The `state` field IS asserted byte-equal. Allow-list: `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset), `hot_restart_version`, `node.user_agent_*`, `node.extensions[]` per-field allow-listed. (Per phase 08.1 SPEC §13.2.) |

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

### 17-name table (introduced by phase 06.1)

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

**Total: 17 internal names.** The four `downstream_rq_Nxx` and four `upstream_rq_Nxx` Prometheus exposition forms collapse to two base-name groups (one HCM, one cluster) per the Rule SN4 status-class flattening discipline.

### Twin-series filter discipline (per empirical-verification scrape)

> **Twin-series filter discipline (per empirical-verification scrape):** Envoy v1.37.2 ALSO emits two twin metric families that envoy-go does NOT emit and the differential fixture (§7) MUST filter out before per-counter delta comparison: (a) `envoy_cluster_external_upstream_rq_xx` (the "external" upstream-rq twin Envoy uses to split internal vs external traffic via `internal_traffic` config); (b) `envoy_listener_http_downstream_rq_xx` (a listener-scoped HCM-rq twin keyed by both listener address and HCM stat_prefix); plus the per-exact-status family `envoy_cluster_upstream_rq{envoy_response_code="200"}` (a separate metric family with `envoy_response_code` label, distinct from `envoy_cluster_upstream_rq_xx`'s `envoy_response_code_class` label). The fixture's allow-list enumerates exactly the 13 unique Prometheus names this SPEC ships; everything else in the Envoy scrape is ignored.

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

---

## Admin API

The envoy-go admin server is a single HTTP/1.1 plaintext bind allocated by `internal/admin.Server.Start()` (per phase 01 contract; reused unchanged in 06.1 and 08.1). Six endpoints are registered on the same `*http.ServeMux`: `/ready` (phase 01), `/stats/prometheus` (phase 06.1), `/config_dump`, `/clusters`, `/listeners`, `/server_info` (phase 08.1). 08.2 will register `POST /drain_listeners` and extend `/ready` + `/server_info` for the DRAINING state.

**Framing deviation (all six admin endpoints).** envoy-go's `net/http` server emits `Content-Length` (the body is buffered before write); upstream Envoy v1.37.2 emits `transfer-encoding: chunked`. The differential harness dechunks upstream responses before byte-comparing the body. This deviation was first documented for `/ready` at phase 01 (per ADR-0015 paragraph 3) and extends unchanged to all six endpoints. No allow-list entry; the dechunk is structural.

**Header set (all six admin endpoints, post-framing-normalization).** The lowercase wire-form header set is `content-type`, `cache-control: no-cache, max-age=0`, `x-content-type-options: nosniff`, `date: <IMF-fixdate>`, `server: envoy` (per ADR-0014). All six endpoints emit this set. The differential harness uses the existing case-insensitive header comparator (introduced for phase 01).

**Method discrimination posture (all six admin endpoints).** Upstream Envoy v1.37.2 does NOT enforce method discrimination on the four 08.1 read-only endpoints (POST/PUT/DELETE return 200 with the same body as GET — empirical pin in 08.1 SPEC §11.8). envoy-go matches Envoy parity (no method check; Go stdlib `http.ServeMux` dispatches on path only). 405 enforcement is deferred to a future security-hardening phase.

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

**Body shape.** `application/json` via the same protojson MarshalOptions as `/config_dump`. Field set populates `version`, `state`, `uptime_current_epoch`, `uptime_all_epochs`, `node` (from bootstrap), partial `command_line_options{config_path}`, `hot_restart_version: "disabled"`. State enum: `LIVE` post-MarkReady, `PRE_INITIALIZING` pre-MarkReady (mathematically complete but unobservable upstream — see SPEC §11.7), `DRAINING` deferred to 08.2, `INITIALIZING` not modeled.

**Empirical evidence (verbatim Envoy v1.37.2 `/server_info`, first 70 lines):** see 08.1 SPEC §11.4.

**Equivalence claim.** Body byte-equal modulo: `version`, `uptime_current_epoch`, `uptime_all_epochs`, `command_line_options.*` (subset on envoy-go side; Envoy emits ~40 fields), `hot_restart_version`, `node.*` (same allow-list as `/config_dump`). The `state` field is byte-equal (`"LIVE"` on both sides).

### Applies to

- phase 08.1 envoy-go admin subsystem.
- all six endpoints: `/ready`, `/stats/prometheus`, `/config_dump`, `/clusters`, `/listeners`, `/server_info`.
- ENVOY_TARGET pin v1.37.2 at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008).

### Does not yet apply to

- HTTP/2 over admin (admin stays HTTP/1.1).
- TLS on admin (admin stays plaintext).
- DRAINING-state response on `/ready` (08.2).
- DRAINING value on `/server_info` `state` field (08.2).
- Mutating endpoints — `POST /drain_listeners` is 08.2; `POST /reset_counters`, `POST /quitquitquit`, `POST /healthcheck/*`, `POST /reopen_logs`, `POST /runtime_modify`, `POST /logging` deferred per ADR-0089.
- JSON form of `/clusters` and `/listeners` — `?format=json` deferred per ADR-0089.
- Query-param filtering on `/config_dump` — `?resource=`, `?mask=`, `?include_eds=` deferred per ADR-0089.
- `RoutesConfigDump`, `SecretsConfigDump`, `ScopedRoutesConfigDump`, `EndpointsConfigDump` envelopes deferred per ADR-0089.
- Other deferred admin endpoints — `/runtime`, `/certs`, `/memory`, `/heap_dump`, `/cpuprofiler`, `/heapprofiler`, `/contention`, `/logging`, `/listeners/<name>/*`, `/init_dump` deferred per ADR-0089.
- ACL / authentication on admin port (no-ACL posture per ADR-0090).
- Method discrimination on read-only endpoints (Envoy parity per SPEC §11.8; 405 enforcement deferred).
- Path normalization beyond Go stdlib `http.ServeMux` (trailing-slash returns Go stdlib `404 page not found`, NOT Envoy's admin help page; allow-listed for trailing-slash behavior — envoy-go's body diverges from Envoy's body, but the status code matches).

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
