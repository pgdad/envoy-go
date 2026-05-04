# envoy-go Roadmap

## Schema

Phases are tracked as rows in the tables below. Columns:

| Column | Meaning |
|---|---|
| `id` | Phase identifier (e.g. `00`, `04`, `04.1`). Sub-phases use a decimal suffix (see §6 of `BOOTSTRAP_PROMPT.md`). |
| `title` | Short human-readable title matching the phase directory slug under `docs/envoy-go/phases/`. |
| `depends-on` | Comma-separated list of phase ids that must be `done` before this phase can enter `in-progress`. Empty for the first phase. |
| `status` | One of `planned`, `in-progress`, `blocked`, `done`. |
| `sub-phases` | If the phase was split (see §6), this lists the sub-phase ids (e.g. `04.1, 04.2`). Empty otherwise. |
| `summary` | One-line summary of the phase's differential surface at completion. |

### Invariants

- **Append-only history.** Rows are never deleted. Only `status` and `sub-phases` columns are updated in place. Sub-phases get their own rows.
- **Status transitions.** `planned` → `in-progress` → (`blocked` or `done`). `blocked` is an exit state only when an upstream dependency is unavailable; the phase resumes on the same row when unblocked.
- **Sub-phase rows.** When a parent phase is split, the parent stays `in-progress` with `sub-phases` listing children; each child gets its own row whose `depends-on` references the prior child (so children execute in order unless stated otherwise).
- **Families (§9 headings) are not rows.** Each heading is expanded into per-phase rows when a phase under that family enters `in-progress`. Do not pre-populate rows for unbrainstormed phases.

---

## MVP Trunk (phases 00–08)

These phases ship in order. Each depends on the previous being `done`, because each adds a primitive the next phase relies on. Splitting (§6) is permitted within any phase.

| id | title | depends-on | status | sub-phases | summary |
|---|---|---|---|---|---|
| 00 | bootstrap | — | done |  | Bootstrap: repo layout, CI, Docker reference Envoy, differential harness skeleton, `ENVOY_TARGET.md` pin, trivial echo fixture. Harness boots; one TCP echo fixture green. |
| 01 | static-bootstrap-config | 00 | done |  | Static bootstrap config loader (node, admin, static_resources skeleton). Config parses; admin `/ready` behaves like Envoy. |
| 02 | tcp-proxy | 01 | done |  | Listener + TCP proxy filter + static cluster + round-robin LB (plaintext). TCP proxy fixture green. |
| 03 | tls | 02 | done |  | Downstream TLS termination + upstream TLS origination + SNI. TLS TCP fixture green. |
| 04 | http-1.1 | 03 | done |  | HTTP connection manager (HTTP/1.1) + route match + router filter + direct_response. HTTP/1.1 routing fixture green. |
| 05 | http-2 | 04 | done | 05.1, 05.2 | HTTP/2 downstream + upstream (low-level framer, own conn mgr). HTTP/2 fixture green; `h2spec` above threshold. Split planner-time per ADR-0045 (`BOOTSTRAP_PROMPT.md` §6.2; phase-05 SPEC §11.1 split-by-surface). |
| 05.1 | downstream-h2 | 04 | done |  | Downstream HTTP/2 termination (own framer, own ServerConn, ALPN dispatch); first non-vacuous conformance gate `h2spec` (53/53 PASS at the ADR-0051 pin); `--allow-h2c` test-only flag; `CONFORMANCE_PINS.md`; `BEHAVIOR_CONTRACT.md ## HTTP/2` scaffold. No new differential fixture (gate a vacuously green per ADR-0045; routed-to-upstream H2 differential surface deferred to 05.2 + fixture 0004). |
| 05.2 | upstream-h2 | 05.1 | done |  | Upstream HTTP/2 origination (`Cluster.DialH2`, own `ClientConn`, `routerActionH2`, cluster `HttpProtocolOptions` parsing); fixture `0004-h2-routing` (full HTTPS h2 end-to-end); closes ADR-0035 H2 leg; extends `BEHAVIOR_CONTRACT.md ## HTTP/2` with upstream + fixture-0004 rules. |
| 06 | observability-baseline | 05 | done | 06.1, 06.2 | Observability baseline: stats + Prometheus admin endpoint + access log (file sink, Envoy default format). Access log + Prometheus fixtures green. Split planner-time per ADR-0045 (`BOOTSTRAP_PROMPT.md` §6.2; phase-06 SPEC + BRAINSTORM §1 split-by-surface). |
| 06.1 | stats-prometheus | 05 | done |  | Internal `stats` package (atomic-counter Registry, lock-free hot path, no third-party Prometheus dependency); `/stats/prometheus` admin endpoint; 17 stat-emit call sites across listener / HCM / cluster (counters + gauges only — histograms deferred); fixture `0005-prometheus-stats` (per-counter delta-equality + per-gauge snapshot-equality on the 17 names against reference Envoy under a 5-request defined load); populates `BEHAVIOR_CONTRACT.md ## Stat-name mapping` (rules SN1–SN8); bundles 05.2 REVIEW M-9 (h2RouterActionAdapter log line). |
| 06.2 | access-log | 06.1 | done |  | Internal `accesslog` package (file sink, Envoy default format formatter, async writer); HCM access-log emit hooks; fixture `0006-access-log`; populates `BEHAVIOR_CONTRACT.md ## Access log field mapping`. Closes the parent row `06` at its phase-done commit (mirrors 05 / 05.1 / 05.2 closure pattern). |
| 07 | filter-chain-framework | 06 | done | 07.1, 07.2 | Filter chain framework: iteration protocol, per-route config, extension registry (HTTP side under HCM); listener-side chain-match completion (listener_filters, FilterChainMatch beyond SNI, default_filter_chain). Framework fixtures green; trivial pluggable filter covers all iteration states. Split planner-time per ADR-0045 (`BOOTSTRAP_PROMPT.md` §6.2; phase-07 BRAINSTORM §1 split-by-surface). |
| 07.1 | http-filter-framework | 06 | done |  | HTTP filter iteration protocol (Envoy-faithful with async-resume; narrow method set; status enums settled) + extension registry (`*HTTPRegistry` threaded; freeze-after-boot mirrors `*stats.Registry` LBP-1) + 3-tier `typed_per_filter_config` merge (Route > VirtualHost > RouteConfiguration; most-specific override) + trivial real filter `envoy.filters.http.cors` (differential fixture `0007a`) + test-only probe filter `envoy.filters.http.envoy_go_test` (structural fixture `0007b`); router moves from `internal/filter/hcm/actions.go` to a new `internal/filter/http/router/` terminal-filter package; populates `BEHAVIOR_CONTRACT.md ## HTTP filter chain` with the four §11 empirical-pin blocks; supersedes ADR-0040 totally; partially supersedes ADR-0042; amends ADR-0041 silent-ignore set. |
| 07.2 | listener-chain-completion | 07.1 | done |  | `listener_filters` framework (`tls_inspector` as the first concrete filter — `original_dst` deferred to a future phase per Decision F) + `FilterChainMatch` fields beyond SNI (destination_port, prefix_ranges, source_*, application_protocols/ALPN, transport_protocol) + `Listener.default_filter_chain` (promoted from phase-03 parse-error). Differential fixture `0008-listener-chain-match` (dual-listener `l_test_a` + `l_test_b` plaintext; 5-connection workload across 4 chains + 1 `default_filter_chain`; per-connection backend-port equivalence). Closes the parent row `07` at its phase-done commit (mirrors 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 closure pattern). Partially supersedes ADR-0033. Anticipated ADRs ADR-0077..ADR-0083 (per SPEC §10). |
| 08 | admin-api-and-drain | 07 | done | 08.1, 08.2 | Minimum admin API (config_dump, stats, clusters, listeners, ready, server_info) + graceful drain. Admin + drain fixtures green. Split planner-time per ADR-0045 (`BOOTSTRAP_PROMPT.md` §6.2; phase-08 BRAINSTORM §1 split-by-surface). |
| 08.1 | admin-endpoints | 07 | done |  | Four new read-only admin endpoints under the existing `internal/admin/` HTTP/1.1 mux: `GET /config_dump` (`application/json` via `protojson` over `*adminv3.ConfigDump`), `GET /clusters` (`text/plain`), `GET /listeners` (`text/plain`), `GET /server_info` (`application/json` via `protojson` over `*adminv3.ServerInfo`); constructor-widening pattern threads `*bootstrap.Bootstrap` + `*cluster.Manager` + `*listener.Manager` into `admin.New()`; new `cluster.Manager.Clusters()` snapshot accessor; `BEHAVIOR_CONTRACT.md ## Admin API — /ready` restructured into `## Admin API` umbrella with five per-endpoint subsections + four new equivalence-matrix rows. Differential fixture `0009-admin-config-dump` (per-endpoint equivalence under a 5-request defined load with per-endpoint tolerance discipline). New fuzzer `FuzzConfigDumpFormat` (10 fuzzers post-08.1). Anticipated ADRs ADR-0084..ADR-0090 (per SPEC §8). Empirical-pin block (SPEC §11) executed IN-SESSION against reference Envoy v1.37.2 per ADR-0004. |
| 08.2 | graceful-drain | 08.1 | done |  | New `internal/drain/` package (drain-state machine LIVE → DRAINING → exit) + `cmd/envoy-go/main.go` SIGTERM-handler upgrade + `internal/listener.Manager.Drain` + `internal/cluster.Manager.Drain` + `POST /drain_listeners` admin endpoint + `/ready` DRAINING-state body extension (partially supersedes ADR-0015) + `/server_info` `state`-field DRAINING transition + `BEHAVIOR_CONTRACT.md ### /drain_listeners` subsection + `## Graceful drain` umbrella section. Differential fixture `0010-graceful-drain` green. Closes the parent row `08` at its phase-done commit (mirrors 05 / 05.1 / 05.2 + 06 / 06.1 / 06.2 + 07 / 07.1 / 07.2 closure pattern). 08.2's phase-done is the BOOTSTRAP_PROMPT.md §8 MVP-trunk-close commit. |
| 09 | http-filter-fault | 08 | done |  | New `internal/filter/http/fault/` package implementing `envoy.filters.http.fault` (Envoy v1.37.2 canonical fault filter) under the 07.1 framework; first concrete phase under §9 HTTP filters family. MVP envelope per SPEC §1 (revised per §11 empirical pins): `delay.percentage` + `delay.fixed_delay` (async-resume via `time.AfterFunc` + `cb.ContinueDecoding`); `abort.percentage` + `abort.http_status` constrained `[200, 600)` per PGV pin §11.1 (terminal-replace via `cb.SendLocalReply` with body byte-exact `fault filter abort` (18 bytes, no trailing newline) + 4-header set per §11.3); `headers` field for request-header gating with StringMatcher.exact only per §11.8; `max_active_faults` concurrency cap (LBP-1 sixth application). Differential fixture `0011-http-fault` green (4 scenarios per SPEC §7.1: delay-only listener-inherited, combined delay+abort per-route, per-route wholesale-override, headers-field exact-match gate; the BRAINSTORM-anticipated 5th "header-driven abort" scenario drops per §11.5 amendment). FIRST production filter exercising async-resume on the request side (cors only short-circuited via SendLocalReply). Deferred per `BOOTSTRAP_PROMPT.md` §9 family-expansion: `response_rate_limit`, `abort.grpc_status` (gRPC family), all four runtime-key fields (Runtime + hot restart family), **`delay.header_delay` + `abort.header_abort` proto sub-messages COUPLED with the four `x-envoy-fault-{delay,abort}-request[-percentage]` request headers** (deferred together per §11.5 + ADR-0104 — the request-header path requires the proto sub-messages to activate; cannot be cleanly separated), `upstream_cluster`, `downstream_nodes`, `disable_downstream_cluster_stats`, `filter_enabled` / `filter_enabled_runtime`, HeaderMatcher non-exact variants (each gets a deferral ADR per ADR-0040 format). 5 stats per §11.6 (4 counters + 1 gauge; 17→22-name BEHAVIOR_CONTRACT extension; route A — `fault.response_rl_injected` permanently-zero counter for parity). Anticipated ADRs ADR-0100..ADR-0107 (per BRAINSTORM §9; ADR-0104 repurposed from implementation to deferral). Per ADR-0106 (BRAINSTORM Decision 12), HTTP filters family expands as flat top-level rows (no parent 09 row with sub-phases); each filter is its own coherent phase. ADR-0045 surface-split release valve stays available if SPEC/PLAN find > ~1500 LoC / > ~25 tasks. |
| 10 | http-filter-header-mutation | 09 | in-progress |  | New `internal/filter/http/header_mutation/` package implementing `envoy.filters.http.header_mutation` (Envoy v1.37.2 canonical header-mutation filter) under the 07.1 framework. THIRD §9 family-row (after cors @ 07.1, fault @ 09). MVP envelope per SPEC §1 (revised per §11 empirical pins): `mutations.request_mutations` + `mutations.response_mutations` (both directions; AppendAction × 4 + Remove + `keep_empty_value` boundary per §11.2 confirmation; multi-valued OVERWRITE-collapse + APPEND-preserve per §11.4 confirmation); `HeaderMutationPerRoute` + `most_specific_header_mutations_wins` (both true/false; multi-tier evaluation across Route/VirtualHost/RouteConfiguration tiers; cross-tier ordering per §11.5 confirmation matches proto comment verbatim). Protected-header set per §11.1 (`{:method, :path, :authority, :scheme, :status, host}` case-insensitive on `host`) — **CONFIG-LOAD-TIME enforcement** (NOT silent runtime no-op as BRAINSTORM hypothesized; MAJOR amendment to BRAINSTORM Decision 11 per §1.1 + ADR-0111). Differential fixture `0012-http-header-mutation` (4 scenarios per SPEC §7.1: listener-only, per-route override, multi-tier flag=false least-specific-wins, multi-tier flag=true most-specific-wins; the BRAINSTORM-anticipated 5th "protected-headers" scenario drops per §11.1 amendment — migrates to unit-test territory since protection is config-load-time). NEW framework method `PerRouteConfig.ResolveAllTiers` (~60 LoC sibling to existing `Resolve` per ADR-0073; amends ADR-0073) + new symmetric `RequestRouteConfigsAllTiers` / `ResponseRouteConfigsAllTiers` callbacks (~30 LoC) + new EAGER per-route validation hook (~40 LoC framework delta per SPEC §6.7 / §12 deferred decision 3). Zero new stats per §11.3 confirmation (`## Stat-name mapping ### 22-name table` UNCHANGED in phase 10; analogous to cors no-stats per ADR-0074). Anticipated ADRs ADR-0108..ADR-0113 (6 ADRs per SPEC §8; ADR-0114 stats-absence dropped per §8.1 consolidation — folded inline into §13.1 BEHAVIOR_CONTRACT subsection). Deferred per ADR-0040 format: `mutations.query_parameter_mutations` (path-query rewriting subsystem; ADR-0112); header-value formatter substitution (`%REQ(:path)%` etc — Envoy command-string subsystem; ADR-0113). Per ADR-0106, §9 family-rows are flat top-level rows; phase 10 lands as row `10`, NOT as a sub-phase of any §9 parent. ADR-0045 surface-split release valve stays available if PLAN finds > ~1500 LoC / > ~25 tasks; SPEC's position is single-row. |

---

## Feature Families (09+)

Each heading below becomes one or more brainstormed phases when it enters `in-progress`. Do **not** pre-populate per-phase rows here; add them at brainstorming time. Splitting (§6) applies as it would to trunk phases.

### HTTP filters family

Header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit.

### Network filters family

redis, mongo, kafka_broker, thrift, zookeeper [scope TBD], echo, direct_response, sni_cluster, rbac network.

### Load balancing family

least_request, random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds.

### Upstream robustness family

Active health checks HTTP/TCP/gRPC/custom, outlier detection variants, circuit breakers, retries + hedging, per-protocol connection pooling.

### HTTP/3 + QUIC family

quic-go transport, downstream H3 listener, upstream H3 cluster, `h3spec` gate.

### gRPC family

gRPC bridge, gRPC-Web, gRPC-JSON transcoding, interop conformance.

### xDS / dynamic config family

ADS, delta xDS, LDS, CDS, RDS, EDS, SDS, RTDS, reconnection, initial-fetch timeout.

### Observability family

gRPC ALS, OTLP access log, OTel/Zipkin/Jaeger/Datadog/XRay tracing, stats sinks, tap filter.

### Runtime + hot restart family

Runtime layer (RTDS consumer); hot-restart / graceful-drain semantics beyond phase 08's minimum.

### WASM host family

Own multi-phase sub-project: ABI, engine binding, proxy-wasm conformance.

### Deprecated / edge features

Explicit out-of-scope ADRs unless later re-opened.
