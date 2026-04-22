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
| 00 | bootstrap | — | planned |  | Bootstrap: repo layout, CI, Docker reference Envoy, differential harness skeleton, `ENVOY_TARGET.md` pin, trivial echo fixture. Harness boots; one TCP echo fixture green. |
| 01 | static-bootstrap-config | 00 | planned |  | Static bootstrap config loader (node, admin, static_resources skeleton). Config parses; admin `/ready` behaves like Envoy. |
| 02 | tcp-proxy | 01 | planned |  | Listener + TCP proxy filter + static cluster + round-robin LB (plaintext). TCP proxy fixture green. |
| 03 | tls | 02 | planned |  | Downstream TLS termination + upstream TLS origination + SNI. TLS TCP fixture green. |
| 04 | http-1.1 | 03 | planned |  | HTTP connection manager (HTTP/1.1) + route match + router filter + direct_response. HTTP/1.1 routing fixture green. |
| 05 | http-2 | 04 | planned |  | HTTP/2 downstream + upstream (low-level framer, own conn mgr). HTTP/2 fixture green; `h2spec` above threshold. |
| 06 | observability-baseline | 05 | planned |  | Access log (file sink, Envoy default format) + stats + Prometheus admin endpoint. Access log + Prometheus fixtures green. |
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
