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
| 07 | filter-chain-framework | 06 | planned |  | Filter chain framework: iteration protocol, per-route config, extension registry. Framework fixtures green; trivial pluggable filter covers all iteration states. |
| 08 | admin-api-and-drain | 07 | planned |  | Minimum admin API (config_dump, stats, clusters, listeners, ready, server_info) + graceful drain. Admin + drain fixtures green. |

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
