# Phase 32 — `redis_proxy` + the upstream connection-pool / cluster-routing seam (parent master SPEC)

> **For agentic workers:** this is the PARENT SPEC for the phase-32 2-way pre-split (32.1 / 32.2). It is NOT directly executable. Per the phase-22 / 25 / 26 / 28 / 29 parent-row precedent, each sub-phase lands its own SPEC → PLAN → IMPL in dedicated sessions. This parent SPEC: (1) resolves the BRAINSTORM §10 D32-1..D32-8 empirical pins IN-SESSION against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` + go-control-plane `/envoy` v1.32.4 (§11), (2) formalizes the 2-way split surface-mapping + per-sub-phase scope boundaries (§3), and (3) anchors the phase-32 ADR-0229 §Context draft (§10). The next session, per BOOTSTRAP §5 + the per-sub-phase precedent, authors the **32.1 SPEC** (which anchors the ADR-0230 §Context for the upstream-pool seam).

**Goal:** Land `envoy.filters.network.redis_proxy` — the project's FIRST terminal routing proxy (it REPLACES `tcp_proxy` as the connection terminator: parses RESP, routes commands `catch_all` to ONE upstream cluster, pools/pipelines the upstream connection itself, and writes RESP replies back downstream) — at a single-route-terminal MVP, across a 2-way feature-progressive pre-split; and land the **upstream connection-pool / cluster-routing seam** (the framework's SIXTH structural extension, ADR-0230) seam-first at 32.1.

**Architecture:** A NEW `internal/filter/network/redisproxy/` package implements `network.TerminalFilter` (NOT `ReadFilter`/`WriteFilter` — it terminates the downstream connection via `Handle(ctx, conn)`, it does not observe a `tcp_proxy`-owned chain): an in-house RESP codec (`resp.go` — decode downstream request frames + decode upstream reply frames + encode; value types `+`/`-`/`:`/`$`/`*` + inline commands) + a command→reply pump that resolves the `catch_all` cluster, round-trips each request through the NEW upstream-pool seam (FIFO/positional reply correlation — RESP carries no correlation id), and writes the reply downstream. The existing `internal/filter/network/` framework gains the upstream connection-pool / cluster-routing extension at 32.1 (ADR-0230): a terminal filter resolves a host of a named cluster via the as-built `cluster.Manager`, dials over the existing `cluster.Cluster.Dial` path (the same `tcp_proxy` uses), pools the conn per upstream host, and offers an in-order request→reply round-trip primitive — redis-scoped (no thrift generalization; YAGNI). The differential proof is TWO-pronged and a §9 FIRST: downstream-RESP-RESPONSE byte-equivalence (redis_proxy GENERATES the downstream response by proxying — it does not pass observed bytes through) PLUS cross-side `StatsAsserter` parity.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy` v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2 `TerminalFilter` seam, 27 ADR-0219 override seam, 29.3 `CloseDirection`), `internal/cluster/` (the `Manager.Get` + `Cluster.Dial` host-resolution + TCP dial path), `internal/filter/tcpproxy/` (the terminal-filter + cluster-dial PRECEDENT the seam parallels), `internal/stats/` (06.1 counters + gauges + `IsValidName`; `internal/stats/name.go` tag-extractor), the differential harness + `StatsAsserter`. **ZERO new go.mod dependencies** (redis_proxy is a CORE `/envoy` extension — UNLIKE kafka_broker's `/contrib`; the RESP codec is in-house byte scanning, no Redis client library).

**Authored:** 2026-06-08. **Empirical-pin probe date:** 2026-06-08.

---

## 1. Mission summary

Phase 32 is the **SIXTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, the phase-28 `zookeeper_proxy` parent, the phase-29 `mongo_proxy` parent, and the phase-31 `kafka_broker` flat row; the phase-30 contrib pin-refresh was an infra row, not a family member) and a parent pre-split row per ADR-0106(d) (the family flat-row discipline preserves ADR-0045's split-gate WITHIN a single filter-row — the zookeeper-28 / mongo-29 in-row split precedent; the project's SIXTH BRAINSTORM-time pre-split after 22/25/26/28/29). It delivers two structurally coupled things:

1. **`envoy.filters.network.redis_proxy`** — the project's **FIRST terminal routing proxy**. Every prior §9 filter either OBSERVED a `tcp_proxy`-owned connection (zookeeper / mongo / kafka_broker) or published an override `tcp_proxy` consumed (sni_cluster); NONE managed an upstream connection. redis_proxy IS the terminator: it implements `TerminalFilter.Handle`, decodes RESP request frames, routes `catch_all` to ONE upstream cluster, round-trips each command through a pooled upstream connection (FIFO/positional correlation), and writes the RESP reply back downstream. There is no `tcp_proxy` behind it → there is **no observational MVP**; the leanest coherent slice is a real proxy that round-trips RESP.
2. **The upstream connection-pool / cluster-routing seam** — the framework's SIXTH structural extension (after the TerminalFilter seam at 26.2, the override seam at 27, the WriteFilter seam at 28.1a, the read seam at 28.1b, and the async halt/resume seam at 29.3): the first seam that lets a network filter ACTIVELY manage an upstream connection (resolve a cluster host, dial, pool, pipeline) rather than observe a `tcp_proxy`-terminated one. It builds ON the existing `TerminalFilter.Handle` seam + the as-built `cluster.Manager`/`cluster.Cluster.Dial` path; its "newness" is the pool + FIFO pipelining + the in-filter routing decision. Landed seam-first at 32.1 (ADR-0230) per the project's seam-first discipline (28.1a/28.1b/29.3).

The design was settled at BRAINSTORM via a 4-question user dialogue (`docs/envoy-go/phases/32-network-filter-redis-proxy/BRAINSTORM.md` §2): Q0 subject = `redis_proxy` (chosen over `thrift_proxy` from {redis, thrift} — RESP is far leaner than Thrift's transport×protocol matrix + method-routing + nested sub-filter chain); Q1 single-route-terminal envelope + 2-way pre-split; Q-split 2-way feature-progressive seam-first; Q-pool-seam framework-level but redis-scoped (YAGNI; thrift reuses/extends later). This SPEC does NOT re-litigate those decisions; it executes the empirical pins they deferred and formalizes the surface-mapping.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D32-1..D32-8 scrape against go-control-plane `/envoy` v1.32.4 + the live contrib reference image + upstream Envoy v1.37.2 source CONFIRMED the SPEC-blocking dep hypothesis (D32-1) and REFINED/REFUTED several BRAINSTORM hypotheses. The probe round-trip ran LIVE (a real `redis:7` backend on a docker bridge network behind the reference redis_proxy returned `+PONG`/`+OK`/`$3\r\nbar`/`:1`/`$-1` replies; `cluster.redis_cluster.upstream_rq_total` 0 → 5). The load-bearing amendments, each carried into the relevant §§ below:

- **AMEND-R1 (D32-1 — TypeURL + ZERO new dep; CONFIRMS BRAINSTORM).** `proto.MessageName(&redis_proxyv3.RedisProxy{})` (run in a throwaway module) = `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` → `@type` = `type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` (carries the `extensions.` segment per `reference_network_filter_typeurl_extensions`). redis_proxy/v3 is a subpackage of the ALREADY-DIRECT `/envoy v1.32.4` module; its transitive imports (`config/core/v3`, `extensions/common/dynamic_forward_proxy/v3`, `cncf/xds/go`, `protoc-gen-validate`) are ALL already in the envoy-go closure → **importing redis_proxy/v3 adds ZERO new go.mod module requirements** (the exact OPPOSITE of kafka_broker's first-`/contrib` dep). Go package alias `redis_proxyv3`. See §5.1 / §11.1.
- **AMEND-R2 (D32-4 — `settings` AND `settings.op_timeout` are PGV-REQUIRED; REFINES the BRAINSTORM "minimum subset" framing; + a RUNTIME missing-upstream reject).** Live `--mode validate` against the contrib image pins THREE PGV hard rejects (prefix `Proto constraint validation failed (...)`): `stat_prefix` min-1; **`settings` is `value is required`** (the whole `ConnPoolSettings` message is mandatory — NOT optional as the BRAINSTORM's "settings minimum subset" implied); **`settings.op_timeout` is `value is required`**. PLUS a distinct RUNTIME (non-PGV) reject — `cannot configure a redis-proxy without any upstream` — when NEITHER `prefix_routes.catch_all_route` NOR `prefix_routes.routes[]` supplies an upstream (fires for `prefix_routes` omitted entirely AND `prefix_routes: {}` empty). Unknown cluster refs are TOLERATED at validate-time (C/K arms). So the MVP config MUST carry `settings: {op_timeout: …}` + a `catch_all_route.cluster`. See §6 / §11.4.
- **AMEND-R3 (D32-2 — the per-command roster is EAGER + TABLE-BOUNDED (~180 commands), NOT dynamic-per-command-seen; REFUTES the BRAINSTORM "likely DYNAMIC" hypothesis; and `enable_command_stats` does NOT gate it in v1.37.2).** The live `/stats` showed ALL ~180 splitter-registered commands present-at-0 from boot (1080 `command.*` lines = ~180 × 6 leaves: `total`/`success`/`error` counters + `latency` histogram + `delay_fault`/`error_fault` fault counters), identical before/after traffic AND identical with `enable_command_stats` true vs false/omitted (the flag toggled neither presence nor increment in this contrib version). Source confirms: `command_splitter_impl.cc addHandler()` pre-creates `command.<lowercased-name>.*` EAGERLY by iterating the static `SupportedCommands` table at config time; a wire command NOT in the table never builds a per-command counter (it routes to `splitter.unsupported_command`). This is the **kafka-EAGER-table-bounded posture, NOT the mongo-dynamic posture** the BRAINSTORM (and ROADMAP rows 32/32.2) hypothesized. envoy-go mirrors EAGER creation at config parse over a static supported-commands table (the kafka-176 precedent); the per-command MVP roster is `total`/`success`/`error` (the `latency` histogram deferred per ADR-0060; the two `*_fault` counters deferred with `faults`). See §7.1 / §7.2.
- **AMEND-R4 (D32-2 — Prometheus exposition is LABEL-HOISTED (mongo/.rbac TAG-EXTRACTOR shape), NOT the kafka INLINE shape).** Live `/stats/prometheus`: redis stats emit as `envoy_redis_<leaf>{envoy_redis_prefix="<stat_prefix>"} <v>` — the metric name is flat `envoy_redis_<leaf>`, the stat_prefix is hoisted into the `envoy_redis_prefix` label (quoted live: `envoy_redis_command_get_total{envoy_redis_prefix="redisprobe"} 2`; `envoy_redis_downstream_cx_total{envoy_redis_prefix="redisprobe"} 8`). This is the **mongo `.rbac.` TAG-EXTRACTOR shape (ADR-0218)** generalized to a `redis.` root — NOT the kafka INLINE shape (`envoy_kafka_<sp>_<rest>{}` empty labels). The `internal/stats/name.go` `redis.` arm follows the mongo/AMEND-B2 precedent: detect `redis.<prefix>.<rest>` → name `envoy_redis_<rest flattened>` + label `envoy_redis_prefix="<prefix>"`. See §7.4.
- **AMEND-R5 (D32-6 SPEC-BLOCKING — PING/AUTH (+ ECHO/TIME/QUIT/HELLO) are answered LOCALLY, ZERO upstream traffic; they count `downstream_rq_total` but emit NO `command.*` counter and NO upstream cx/rq).** Live-confirmed: a PING-only connection returned `+PONG\r\n` with `cluster.redis_cluster.upstream_cx_total`/`upstream_rq_total` staying 0 and `redis.<sp>.downstream_rq_total` incrementing; PING with an argument still returns `+PONG\r\n` (arg ignored — does NOT echo, unlike a real redis server). AUTH with NO `downstream_auth_password` configured returns `-ERR Client sent AUTH, but no password is set\r\n` locally (zero upstream). Source: `command_splitter_impl.cc` + `proxy_filter.cc onAuth()` handle PING/AUTH/ECHO/TIME/QUIT/HELLO in-filter; only data commands (SET/GET/INCR/…) proxy via `makeRequest()`. The MVP minimal local-reply set is **PING (+PONG) + AUTH (error-when-no-auth)**. This is BLOCKING because it determines the `0055` fixture's exercised commands, the `TCPRedisResponder` expected traffic, and the round-trip stat counts. See §7.3 / §8.1 / §11.6.
- **AMEND-R6 (D32-2 — the upstream/pool stat surface is SPLIT across two scopes; the `upstream_cx_*`/`upstream_rq_*` traffic stats live under the existing CLUSTER scope, NOT a new redis roster).** Source + live: redis_proxy defines only 3 redis-specific pool counters (`REDIS_CLUSTER_STATS`) under the filter scope `redis.<sp>.` — `upstream_cx_drained`, `max_upstream_unknown_connections_reached`, `connection_rate_limited`. The standard `upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total`/… are the cluster's own traffic stats under `cluster.<name>.*` (incremented via `host_->cluster().trafficStats()`), NOT defined by redis_proxy. envoy-go ALREADY registers a per-cluster roster (`cluster.<name>.upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total`/`upstream_rq_2xx..5xx`/`membership_total`, manager.go:97-108) — so the seam reuses those for upstream traffic; the redis HTTP-shaped `upstream_rq_2xx..5xx` have no RESP analog (per-side asymmetry, pinned where the reference diverges). See §7.2 / §7.5.
- **AMEND-R7 (D32-2 — the fixed `redis.<sp>.` roster is 11 counters + 4 GAUGES + 2 splitter counters + the 3 REDIS_CLUSTER_STATS).** `ALL_REDIS_PROXY_STATS` = 6 counters (`downstream_cx_total`/`downstream_cx_drain_close`/`downstream_cx_protocol_error`/`downstream_cx_rx_bytes_total`/`downstream_cx_tx_bytes_total`/`downstream_rq_total`) + 4 GAUGES Accumulate (`downstream_cx_active`/`downstream_cx_rx_bytes_buffered`/`downstream_cx_tx_bytes_buffered`/`downstream_rq_active`) — the project's SECOND mirrored gauge family after mongo's `op_query_active`. Plus 2 splitter counters (`splitter.invalid_request`/`splitter.unsupported_command`) + the 3 REDIS_CLUSTER_STATS = **15 fixed names under `redis.<sp>.`** (11 counters + 4 gauges). See §7.2.
- **AMEND-R8 (D32-3 — the seam model: ONE shared client per HOST, multiplexed; FIFO positional pop-front correlation).** Source: the conn pool keeps `client_map_[host] → one ClientImpl` (one upstream connection per host, shared across all downstream conns, thread-local); each `ClientImpl` owns a `std::list<PendingRequest> pending_requests_` — send `emplace_back`, reply `pending_requests_.front()` + `pop_front()` (positional). Replies route back to the right downstream via the per-request callbacks, NOT connection identity. envoy-go's redis-scoped MVP may use the simplest faithful model (**one upstream conn per downstream conn** — trivially-correct positional correlation, no cross-downstream multiplexing); the FIFO+positional contract is the load-bearing invariant either way. The exact multiplexing model + the seam's exported API are pinned at the 32.1 SPEC (D-P32-3). See §4.1.
- **AMEND-R9 (D32-8 — NO close-direction counters; redis_proxy stays close-direction-framework-zero-touch).** `ALL_REDIS_PROXY_STATS` has no `cx_destroy_local/remote`-style direction-keyed counter (contrast mongo's `cx_destroy_local/remote_with_active_rq`) — only `downstream_cx_drain_close` (a DRAIN counter, gated on the drain decision, not close direction) + `upstream_cx_drained`. The 29.3 `CloseDirection` machinery is NOT needed by phase 32. The `reference_close_direction_framework_gap` lesson does NOT bite here. See §11.8.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0228** (the kafka_broker filter); next-free **ADR-0229**. This parent SPEC anchors the **ADR-0229 §Context draft** (the redis_proxy filter + the parent-row umbrella) into DECISIONS.md — tail ADR-0228 → ADR-0229; next-free → ADR-0230. Per the BRAINSTORM §7 + the next-prompt §11, **ADR-0230 (the upstream-pool seam) anchors its §Context at the 32.1 SPEC, NOT here** (this differs from the phase-28/29 precedent of drafting all sub-phase ADRs at the parent SPEC — the seam's exact API is a 32.1-SPEC pin, so its §Context is authored where the design lands). ADR-0229's §Decision/§Consequences bodies land at the respective IMPL sub-phases per ADR-0044. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. All eight D32 pins are RESOLVED this session (§11); the remaining open items are sub-phase-SPEC/PLAN D-questions (§12), not empirical pins.

---

## 2. Scope — non-purposes + REUSES-not-consumed

### 2.1 Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **`prefix_routes` multi-cluster routing beyond `catch_all`** — route-by-key-prefix to multiple clusters (`routes[].prefix`/`remove_prefix`/`case_insensitive`, per-route `request_mirror_policy`, `read_command_policy`). The `catch_all_route.cluster` leg is the MVP; `prefix_routes.routes[]` is PARSE-ACCEPTED-behavior-DEFERRED (the prefix table extends the seam's routing decision). The MVP requires `catch_all_route` (the missing-upstream runtime reject, AMEND-R2).
- **Hash-ring sharding + `enable_redirection` (MOVED/ASK)** + `enable_hashtagging` + `dns_cache_config` — Redis Cluster-mode host selection by key hash + redirection handling. `settings.enable_redirection`/`enable_hashtagging` PARSE-ACCEPTED-behavior-DEFERRED (defaults off; validate-accepts even without `dns_cache_config`, AMEND-R2 arm L). A future routing sub-phase.
- **Multi-key command fragmentation (MGET/MSET/DEL split-and-collate)** — splitting a multi-key command across shards + collating the replies; introduces fan-out/collate that breaks the trivial FIFO pending queue (§4.1). A future sub-phase.
- **Downstream AUTH enforcement** (`downstream_auth_password`/`downstream_auth_passwords`/`downstream_auth_username`/`external_auth_provider`) — PARSE-ACCEPTED-behavior-DEFERRED (each validate-accepts standalone, AMEND-R2 arms J/K). The MVP answers AUTH locally with the no-password-set error (AMEND-R5); a future auth sub-phase wires the configured-password path + the gRPC external-auth provider.
- **`faults`** (`RedisFault[]` error/delay fault injection) — PARSE-ACCEPTED-behavior-DEFERRED (validate-accepts standalone, AMEND-R2 arm I). The per-command `delay_fault`/`error_fault` counters (AMEND-R3) defer with it. A future sub-phase (the redis analogue of the mongo fault-delay; may consume the ADR-0226 async halt machinery or a redis-local timer).
- **Request mirroring** (`routes[].request_mirror_policy`) — a future sub-phase.
- **Replica `read_policy`** (`settings.read_policy` — route reads to replicas) — PARSE-ACCEPTED-behavior-DEFERRED (PGV `defined_only`; default MASTER). A future sub-phase (extends host selection).
- **The full `ConnPoolSettings` surface** (`max_buffer_size_before_flush`/`buffer_flush_timeout` flush-batching; `max_upstream_unknown_connections`; `connection_rate_limit`) — the MVP consumes only the required `op_timeout`; the rest parse-accepted-deferred. Their stat counters (`max_upstream_unknown_connections_reached`/`connection_rate_limited`) exist-at-0.
- **Command-latency histograms** (`command.<cmd>.latency`) — deferred per ADR-0060 (project-wide histogram deferral); coverage-boundary record.
- **`enable_command_stats` gating semantics** — in v1.37.2 the flag gates NEITHER per-command-counter presence NOR increment (AMEND-R3); envoy-go treats per-command stats as always-on (the flag parse-accepted, behavior a no-op-for-counter-presence — a coverage-boundary note). What (if anything) the flag still gates in contrib-v1.37.2 is not observable from the stat roster.
- **Runtime-key gating** — envoy-go has no runtime layer; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot restart family row is the future home).
- **Real-Redis-server integration fixtures** — out of scope; the hermetic synthesized `TCPRedisResponder` only (a real Redis server adds container weight + RESP3/banner nondeterminism — the probe used `redis:7` ONLY to pin the reference, not as a fixture backend).
- **RESP3 (`HELLO 3` protocol negotiation)** — the reference rejects RESP3 with `NOPROTO unsupported protocol version` (probe caveat); envoy-go MVP speaks RESP2; HELLO is answered locally.
- **The remaining protocol proxy** — `thrift` — its own future family phase (transport×protocol matrix + method-routing + nested thrift-filter sub-chain). Thrift REUSES/extends the 32.1 upstream-pool seam.

### 2.2 REUSE-by-absence: no per-route surface

Network filters carry no `typed_per_filter_config` surface (the phase-26/29/31 confirmation; re-confirmed by absence — the RedisProxy proto has no `*PerRoute` message). redis_proxy's `prefix_routes` is its OWN internal routing table (not the HTTP route-config `typed_per_filter_config` mechanism); only the `catch_all_route` leg is consumed at the MVP. The ADR-0125 roster is untouched.

### 2.3 REUSES (not new primitives)

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b/29.3) — the `TerminalFilter` seam (`Handle(ctx, conn)`, terminal.go), the registry, the `builtins.RegisterBuiltins` seam, the chain runtime, the ADR-0219 override seam (upstreamcluster.go — the nearest routing precedent), the freeze-after-boot registry discipline. The NEW upstream-pool seam (§4.1) lands IN this package.
- `internal/cluster/` (02/05.2/06.1) — `Manager.Get(name)` host-resolution + `Cluster.Dial(ctx)` (the TCP dial path the same `tcp_proxy` uses, filter.go:127) + the per-cluster traffic-stat roster (`cluster.<name>.upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total`/`membership_total`, manager.go:97-108 — the upstream-traffic-stat source, AMEND-R6). The upstream-pool seam builds ON `Manager.Get` + `Cluster.Dial`.
- `internal/filter/tcpproxy/` (02/26.2/27) — the existing terminal-filter that ALSO dials a cluster (filter.go `Handle` — the dial + bidirectional-pump PRECEDENT; redis_proxy is a different terminal that dials the SAME way but pools + RESP-pipelines instead of raw-copying). Untouched by 32.
- `internal/stats/` (06.1) — counters + **gauges** (the second mirrored-gauge consumer after mongo's `op_query_active`) + `NewCounterIfAbsent` (eager-roster idempotent across listeners sharing a `stat_prefix` — the kafka/mongo precedent) + `IsValidName` (the config-boundary `stat_prefix` guard + the codec-boundary command-name guard if a dynamic path is chosen, D-P32-7); `internal/stats/name.go` (the NEW `redis.` TAG-EXTRACTOR arm, AMEND-R4).
- `internal/filter/network/kafkabroker/` (31) + `internal/filter/network/mongoproxy/` (29) — the §9 package-shape precedents `redisproxy` mirrors (two-step factory ADR-0079; the eager-roster `newKafkaStats`/`newMongoStats` shape; in-house byte-scanning codec).
- The differential harness + `fixture.StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints) — booting the contrib reference image (ADR-0227).
- `envoy.extensions.filters.network.redis_proxy.v3` proto bindings (go-control-plane `/envoy` v1.32.4 — ALREADY a dep, AMEND-R1; the `redis_proxy/v3` blank-import added to `internal/bootstrap/bootstrap.go`).
- The two-step factory (ADR-0079), atomic landing + six-gate (ADR-0052), byte-stable PARSE-REJECT wording (ADR-0080).
- **NOT consumed:** the ADR-0221 `network.WriteFilter` seam (a terminal routing proxy owns both connection ends directly — it does not observe a `tcp_proxy`-terminated write chain; the upstream-pool seam is its analogue); the ADR-0226 async halt/resume seam (the MVP injects no fault delay → no halt); the 29.3 `CloseDirection` machinery (no close-direction counters, AMEND-R9); the ADR-0217 dynamic-metadata Bucket (redis_proxy emits none); histograms (the `command.<cmd>.latency` family deferred, ADR-0060).

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q-split

The 2-way FEATURE-PROGRESSIVE seam-first pre-split was settled at BRAINSTORM Q-split (ROADMAP rows 32.1/32.2 already `planned`). No SPEC-time re-decision. The D32-8 envelope (§11.8 + §15) confirms each sub-phase fits the ADR-0045 gate individually (~1100-1550 production LoC across both → ~600-800 each). Each sub-phase re-checks at its own PLAN; a 32.x escape-valve split stays available.

### 3.1 Split surface-mapping table (per phase-22/25/26/28/29 §3.1 precedent)

| Surface element | 32.1 | 32.2 |
|---|---|---|
| `internal/filter/network/redisproxy/` package + TypeURL + config parse (`stat_prefix` + `settings.op_timeout` + `prefix_routes.catch_all_route.cluster`) | **lands** | — |
| The upstream connection-pool / cluster-routing SEAM (ADR-0230; §4.1) in `internal/filter/network/` | **lands** | — |
| In-house RESP codec (`resp.go`; `+`/`-`/`:`/`$`/`*` + inline + null `$-1`/`*-1`) — request decode + reply decode + encode | **lands** | — |
| `TerminalFilter.Handle` command→reply pump (read request → route catch_all → seam round-trip → write reply) | **lands (single-command)** | **full command set** |
| PING/AUTH local-reply (AMEND-R5; `+PONG` / no-password error) | **lands (PING + AUTH)** | + ECHO/TIME/QUIT/HELLO (D-P32-6) |
| The fixed 15-name roster under `redis.<sp>.` (11 counters + 4 gauges; §7.2) — CREATION | **lands (downstream_cx/rq subset for the round-trip)** | **full roster** |
| The 4 downstream GAUGES (`downstream_cx_active`/`downstream_rq_active`/rx+tx buffered) | created 32.1 | inc/dec lands 32.2 |
| The 3 REDIS_CLUSTER_STATS (`upstream_cx_drained`/`max_upstream_unknown_connections_reached`/`connection_rate_limited`) | — | **lands** |
| The 2 splitter counters (`splitter.invalid_request`/`splitter.unsupported_command`) | — | **lands** |
| The EAGER per-command roster (`command.<cmd>.{total,success,error}` over the static ~180-cmd table; AMEND-R3) | — | **lands** |
| The `redis.` TAG-EXTRACTOR arm in `internal/stats/name.go` (AMEND-R4) | — | **lands** |
| `IsValidName` charset disposition (table-bounded by-construction vs codec-boundary guard; D-P32-7) | — | **lands** |
| 10th `builtins.RegisterBuiltins` registration + `redis_proxy/v3` blank-import in `bootstrap.go` | **lands** | — |
| New BackendKind (`TCPRedisResponder`, anticipated value 32) | **lands** | extended (command matrix) |
| Differential fixtures | +1 (`0055-redis-roundtrip` cross-side, round-trip arm) + `0056-redis-boot-reject` if pinned here | extends `0055` (command matrix) |
| New fuzzer (`FuzzRESPDecode`, 41st) | — | **lands** |
| BEHAVIOR_CONTRACT bundle | 32.1 bundle | 32.2 bundle (+ histogram/faults/runtime coverage boundaries) |
| Anticipated ADRs | 0230 §Context at the 32.1 SPEC; §Decision/§Consequences body at the 32.1 IMPL | 0229 §Decision/§Consequences body |
| ROADMAP | 32.1 `planned → in-progress` at the 32.1 SPEC; `→ done` at the 32.1 IMPL | 32.2 same; **parent row 32 ROLLUP `→ done` ATOMICALLY with 32.2** |

### 3.2 Per-sub-phase scope detail

**32.1 `network-filter-redis-upstream-pool-seam-and-codec`** — (a) the **upstream connection-pool / cluster-routing SEAM** (§4.1; ADR-0230) in `internal/filter/network/` — a redis-scoped exported primitive that resolves a host of a named cluster via `cluster.Manager.Get`, dials via `cluster.Cluster.Dial`, pools the conn per upstream host (or the simplest one-conn-per-downstream model, D-P32-3), and offers an in-order (FIFO/positional) request→reply round-trip; built on `TerminalFilter.Handle`; the exact API + multiplexing model pinned at the 32.1 SPEC; (b) the **`internal/filter/network/redisproxy/` package foundation** — TypeURL via `proto.MessageName` (§5.1; the `extensions.` lesson), config parse of `stat_prefix` (PGV-required → boot-reject) + `settings.op_timeout` (PGV-required → boot-reject) + `prefix_routes.catch_all_route.cluster` (PGV-required min-1 → boot-reject; + the runtime missing-upstream reject, AMEND-R2) with the rest of the proto parse-accepted/deferred (D32-4), the **RESP codec** (`resp.go` — request decode + reply decode + encode; value types `+`/`-`/`:`/`$`/`*` + null `$-1`/`*-1` + inline commands; the wire framing adopted VERBATIM, §11.5), the `TerminalFilter.Handle` command→reply pump (read RESP request → PING/AUTH local-reply OR route catch_all → seam round-trip → write RESP reply downstream), and a minimal `catch_all` single-command round-trip (PING [local] + GET/SET [proxied] — D-P32-6) proving the seam end-to-end; (c) registration as the **10th built-in** + the `redis_proxy/v3` blank-import in `bootstrap.go` (ZERO new go.mod dep); (d) the new BackendKind (the synthesized `TCPRedisResponder` — §8.3); (e) fixtures **`0055-redis-roundtrip`** (cross-side; the round-trip arm + downstream-RESP byte-equivalence) + **`0056-redis-boot-reject`** if the parent-SPEC pins the required-field arms there (D-P32-5); (f) the ADR-0230 §Context (at the 32.1 SPEC) + §Decision/§Consequences body (at the 32.1 IMPL); the BEHAVIOR_CONTRACT 32.1 bundle; STATE/ROADMAP advance (sub-row 32.1 `→ done`).

**32.2 `network-filter-redis-commands-and-stats`** — (a) the **full single-route command set** (the core single-key commands routed via catch_all) + the remaining local-reply commands (ECHO/TIME/QUIT/HELLO, D-P32-6) on top of the 32.1 seam; (b) the **per-command + downstream + upstream stat roster** — the EAGER `command.<cmd>.{total,success,error}` over the static ~180-command table (AMEND-R3; the `latency` histogram + `*_fault` counters deferred) + the 4 downstream gauges' inc/dec + the 2 splitter counters + the 3 REDIS_CLUSTER_STATS, all under `redis.<stat_prefix>.`; the new `redis.` TAG-EXTRACTOR arm at `internal/stats/name.go` (AMEND-R4); the `IsValidName` disposition (D-P32-7 — table-bounded eager → satisfied by construction, OR a codec-boundary guard if a dynamic path is chosen); (c) the **differential command matrix** (a command spread + error/edge arms: GET-hit/GET-miss [`$-1` null bulk], SET, INCR, an unsupported command → `splitter.unsupported_command`, a bad-arity command → `splitter.invalid_request`/`-invalid request`) extending `0055`; (d) the **fuzzer** `FuzzRESPDecode` (the 41st — no-panic / no-mutation / bounded-buffer on the RESP decoder); (e) the BEHAVIOR_CONTRACT 32.2 bundle + the ADR-0229 §Decision/§Consequences body + the **parent-row-32 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 32.2 per the 18/19/22/24/25/26/28/29 precedent) + the six-gate.

---

## 4. Framework primitives — 1 NEW framework SEAM (32.1) + 1 NEW filter package (32.1/32.2)

### 4.1 NEW: the upstream connection-pool / cluster-routing seam (ADR-0230; lands at 32.1)

In the existing `internal/filter/network/` package (NOT a new package). The as-built anchor points it builds on (verified this session against the worktree tip):

- `internal/filter/network/terminal.go:30-49` — the `TerminalFilter` interface (`Handle(ctx, downstream net.Conn)`); redis_proxy IS a terminal filter (UNCHANGED signature).
- `internal/cluster/manager.go:110-114` — `Manager.Get(name) (*Cluster, bool)` host-resolution lookup (the same `tcp_proxy.NewFilter` uses at filter.go:63).
- `internal/cluster/` `Cluster.Dial(ctx) (net.Conn, _, error)` — the TCP dial path (the same `tcp_proxy.Handle` uses at filter.go:127); the upstream-pool seam dials over it.
- `internal/filter/tcpproxy/filter.go:101-159` — the terminal dial + bidirectional-pump PRECEDENT (redis_proxy replaces the raw `io.Copy` pump with a RESP request→reply round-trip over a pooled conn).
- `internal/filter/network/upstreamcluster.go` — the ADR-0219 override-seam shape (the nearest in-filter-routing-decision precedent; redis_proxy's catch_all routing is its analogue, internal to the filter).

The upstream model pinned from source (AMEND-R8 / §11.3):

1. **Routing.** `route (longest-prefix) → catch_all → cluster name`. MVP: `catch_all_route.cluster` → ONE cluster (no prefix table).
2. **Host resolution + dial.** `cluster.Manager.Get(name) → choose a host → cluster.Cluster.Dial(ctx)`. (Upstream uses `cluster_->loadBalancer().chooseHost(lb_context)`; envoy-go's `Cluster.Dial` already encapsulates round-robin host selection — the seam REUSES it, no new LB surface.)
3. **Pooling + multiplexing.** Upstream keeps ONE shared `ClientImpl` per host (one upstream connection, multiplexed across all downstream conns, thread-local). envoy-go's redis-scoped MVP MAY use the simplest faithful model — **one upstream conn per downstream conn** (no cross-downstream multiplexing; trivially-correct positional correlation) — and pin `upstream_cx_*` per-side where the reference's pooling internals diverge (§7.5). The exact multiplexing model is the 32.1 SPEC's D-P32-3.
4. **Reply correlation (the load-bearing invariant).** RESP carries NO correlation id — replies return in REQUEST ORDER (pipelined). Per upstream connection: a FIFO pending-request queue; each reply dequeues the OLDEST pending request (pop-front, positional) and is routed back to that request's originating downstream. The MVP's single-route single-key-command envelope keeps the queue trivially in-order (no fan-out/collate — only multi-key fragmentation would introduce it, deferred).

Anticipated near-minimal new exported surface (a small pool/round-trip type the redisproxy terminal calls); the exact API is pinned at the 32.1 SPEC (D-P32-3). Redis-scoped (Q-pool-seam): catch_all single-cluster routing, one logical request stream per downstream connection, basic per-host pooling — NO thrift-specific generalization (no method-routing abstraction, no pluggable-codec interface). Thrift reuses/extends it at its phase (the extract-at-second-consumer discipline). `TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM untouched; existing fixtures byte-identical (the seam is additive — it only activates for a redis_proxy terminal).

### 4.2 NEW: `internal/filter/network/redisproxy/` (ADR-0229; lands across 32.1/32.2)

Go package `redisproxy` (single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy`/`kafkabroker` precedent). Implements `network.TerminalFilter` (one instance per chain; per-connection state lives on `Handle`'s stack or in a per-connection struct). Anticipated layout (the 32.x SPECs/PLANs finalize the file split):

- `redisproxy.go` — TypeURL (via `proto.MessageName(&redis_proxyv3.RedisProxy{})`, pinned by an IMPL Task-1 test, NEVER hand-typed) + `NewFactory`.
- `config.go` — the config parse (`stat_prefix` + `settings.op_timeout` + `prefix_routes.catch_all_route.cluster` PGV arms + the runtime missing-upstream check + the deferred-field parse-accept) + `stats.IsValidName(stat_prefix)` config-boundary guard.
- `resp.go` — the in-house RESP codec (request decode + reply decode + encode; value types + null sentinels + inline commands; decoder-internal partial-frame reassembly).
- `stats.go` — the eager fixed roster (11 counters + 4 gauges + 2 splitter + 3 REDIS_CLUSTER_STATS) + the eager per-command roster over the static supported-commands table + the inc accessors.
- `commands.go` — the static supported-commands table (~180 commands; D-P32-2) + the local-reply command set (PING/AUTH/ECHO/TIME/QUIT/HELLO).
- `filter.go` — the `TerminalFilter.Handle` command→reply pump + the upstream-pool seam consumption + the downstream gauges.

The RESP codec lives INSIDE the package (`resp.go` — NOT a new top-level package; extract-at-second-consumer; YAGNI). The upstream-pool seam lands IN `internal/filter/network/` (§4.1).

### 4.3 Framework-delta accretion shape

Phase 32 continues framework GROWTH: the upstream connection-pool / cluster-routing seam is the framework's SIXTH structural extension and the FIRST that lets a network filter ACTIVELY manage an upstream connection. It builds on the as-built `TerminalFilter.Handle` + `cluster.Manager`/`Cluster.Dial` — its newness is the pool + FIFO pipelining + the in-filter routing decision, NOT a new connection primitive. The deferred sharding/redirection/fragmentation/mirroring surface (§2.1) WOULD each extend this seam, but they are deferred (the consume-at-consumer discipline).

---

## 5. Proto-field roster (per §11.1 D32-1 + §11.4 D32-4)

All rosters transcribed from go-control-plane `/envoy` v1.32.4 (`extensions/filters/network/redis_proxy/v3/redis_proxy.pb.go` + `.pb.validate.go`); verified by `proto.MessageName` run in-session.

### 5.1 TypeURL

`proto.MessageName(&redis_proxyv3.RedisProxy{})` = `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` → **`@type` = `type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy`** (the `extensions.` segment per `reference_network_filter_typeurl_extensions`; pinned by an IMPL Task-1 `proto.MessageName` test, NEVER the docs string). The filter registration name (the listener filter-chain `name`) is `envoy.filters.network.redis_proxy`. Go package alias `redis_proxyv3`; CORE `/envoy` (NOT `/contrib`) — ZERO new go.mod dep (AMEND-R1).

### 5.2 `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` (top-level; tags 1,3-10, tag 2 absent)

| Go field | proto field | tag | Go type | PGV | 32.x disposition |
|---|---|---|---|---|---|
| `StatPrefix` | `stat_prefix` | 1 | `string` | **required min_len 1 rune** | REQUIRED → boot-reject (32.1; the `0056` fixture arm) |
| `Settings` | `settings` | 3 | `*RedisProxy_ConnPoolSettings` | **required (`value is required`)** + recurse | REQUIRED → boot-reject (32.1; AMEND-R2) |
| `LatencyInMicros` | `latency_in_micros` | 4 | `bool` | none | parse-accept (latency-histogram unit; histograms deferred) |
| `PrefixRoutes` | `prefix_routes` | 5 | `*RedisProxy_PrefixRoutes` | not PGV-required (recurse) | catch_all leg consumed 32.1; routes[] parse-accept-deferred; RUNTIME missing-upstream reject (AMEND-R2) |
| `DownstreamAuthPassword` | `downstream_auth_password` | 6 | `*core.DataSource` | DEPRECATED; recurse | parse-accept-deferred |
| `DownstreamAuthUsername` | `downstream_auth_username` | 7 | `*core.DataSource` | recurse | parse-accept-deferred |
| `Faults` | `faults` | 8 | `[]*RedisProxy_RedisFault` | repeated; recurse | parse-accept-deferred |
| `DownstreamAuthPasswords` | `downstream_auth_passwords` | 9 | `[]*core.DataSource` | repeated; recurse | parse-accept-deferred |
| `ExternalAuthProvider` | `external_auth_provider` | 10 | `*RedisExternalAuthProvider` | recurse | parse-accept-deferred |

### 5.3 `RedisProxy_ConnPoolSettings` (settings; tags 1-10)

| Go field | proto field | tag | Go type | PGV | 32.x disposition |
|---|---|---|---|---|---|
| `OpTimeout` | `op_timeout` | 1 | `*durationpb.Duration` | **required (`value is required`)** | REQUIRED → boot-reject (32.1; AMEND-R2) |
| `EnableHashtagging` | `enable_hashtagging` | 2 | `bool` | none | parse-accept-deferred (sharding) |
| `EnableRedirection` | `enable_redirection` | 3 | `bool` | none | parse-accept-deferred (MOVED/ASK) |
| `MaxBufferSizeBeforeFlush` | `max_buffer_size_before_flush` | 4 | `uint32` | none | parse-accept-deferred (flush-batching) |
| `BufferFlushTimeout` | `buffer_flush_timeout` | 5 | `*durationpb.Duration` | recurse | parse-accept-deferred |
| `MaxUpstreamUnknownConnections` | `max_upstream_unknown_connections` | 6 | `*wrapperspb.UInt32Value` | recurse (doc default 100) | parse-accept-deferred |
| `ReadPolicy` | `read_policy` | 7 | enum `ReadPolicy` | **`defined_only`** (default MASTER=0) | parse-accept-deferred (replica reads) |
| `EnableCommandStats` | `enable_command_stats` | 8 | `bool` | none (default false) | parse-accept; NO-OP for counter presence in v1.37.2 (AMEND-R3) |
| `DnsCacheConfig` | `dns_cache_config` | 9 | `*dfp.DnsCacheConfig` | recurse | parse-accept-deferred |
| `ConnectionRateLimit` | `connection_rate_limit` | 10 | `*RedisProxy_ConnectionRateLimit` | recurse | parse-accept-deferred |

### 5.4 `RedisProxy_PrefixRoutes` + `RedisProxy_PrefixRoutes_Route`

`PrefixRoutes`: `routes` (1, `[]*Route`, recurse — parse-accept-deferred), `case_insensitive` (2, bool — parse-accept-deferred), `catch_all_route` (4, `*Route`, not PGV-required at the message level — the MVP leg). `Route`: `prefix` (1, string, `max_len 1000`, no min — empty allowed for catch_all), `remove_prefix` (2, bool), **`cluster` (3, string, required min_len 1)** — the catch_all's `cluster` is the load-bearing MVP field (a PGV reject if empty), `request_mirror_policy` (4, repeated — deferred), `key_formatter` (5, string — deferred), `read_command_policy` (6, message — deferred). The "catch_all-required-when-no-routes" rule is NOT PGV — it is the RUNTIME `cannot configure a redis-proxy without any upstream` reject (AMEND-R2 / §6.2).

---

## 6. PARSE-REJECT roster (per §11.4 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a table test at IMPL. Boot-reject PARITY arms (mirroring an upstream PGV/config-load failure) are distinguished from envoy-go-strict DEPARTURE arms. Phase 32 has NO departure-class rejects — every reject below mirrors an upstream reject; the departures in this phase are all behavioral (deferred active features; histogram deferral; runtime keys at defaults; enable_command_stats no-op), recorded in BEHAVIOR_CONTRACT, never as rejects. NOTE the C++ vs Go PGV idiom difference (`value length must be at least 1 characters` C++ vs `1 runes` Go) — envoy-go's reject wording is its OWN ADR-0080 constant; the boot-reject differential checks BOTH sides reject at boot (a boot-stderr substring matched per-side), not exact cross-impl string equality (the kafka `0054`/mongo `0050` precedent).

### 6.2 32.1 PARSE-REJECT arms (all parse code lands at 32.1)

- `redis-proxy-stat-prefix-required` — boot-reject PARITY (the `stat_prefix` PGV min-1-rune rule, §5.2). The load-bearing `0056` fixture arm. Reference C++ wording: `Proto constraint validation failed (RedisProxyValidationError.StatPrefix: value length must be at least 1 characters)`.
- `redis-proxy-settings-required` — boot-reject PARITY (the `settings` PGV `value is required` rule, AMEND-R2). Reference: `Proto constraint validation failed (RedisProxyValidationError.Settings: value is required)`.
- `redis-proxy-op-timeout-required` — boot-reject PARITY (the `settings.op_timeout` PGV `value is required` rule, AMEND-R2). Reference: `… ConnPoolSettingsValidationError.OpTimeout: value is required`.
- `redis-proxy-no-upstream` — boot-reject PARITY but a RUNTIME (non-PGV) check (NEITHER `catch_all_route` NOR `routes[]` supplies an upstream). Reference: `cannot configure a redis-proxy without any upstream` (NO `Proto constraint validation failed` prefix). Fires for `prefix_routes` omitted AND `prefix_routes: {}` empty.
- `redis-proxy-catch-all-cluster-required` — boot-reject PARITY (the `catch_all_route.cluster` PGV min-1 rule when a catch_all is present with an empty cluster, §5.4).
- Framework-level: unknown network-filter `typed_config` type_url → existing boot-reject (no new arm).
- Unknown cluster ref in `catch_all_route.cluster` / `external_auth_provider.grpc_service` → NOT a reject at config time (validate-accepts, AMEND-R2 arms C/K — the subject must NOT reject unknown-cluster refs at parse).

Which of these gain `0056` FIXTURE arms (vs unit-test-only) is D-P32-5 (anticipated: `0056` carries the `stat_prefix` arm — the load-bearing required-field arm — per the kafka `0054`/mongo `0050` precedent; the `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` arms unit-tested).

---

## 7. Stat surface (per §11.2 D32-2 + AMEND-R3/R4/R6/R7)

### 7.1 Scope/naming — `redis.<stat_prefix>.<leaf>` (AMEND-R7)

Upstream: the filter stats live under `redis.<stat_prefix>.` (probed live: `redis.redisprobe.downstream_cx_total`, `redis.redisprobe.command.get.total`, `redis.redisprobe.splitter.unsupported_command`). The per-command segment is `command.<lowercased-wire-name>.<leaf>` (built from `absl::AsciiStrToLower(request->asArray()[0].asString())` but only for names in the static supported-commands table — AMEND-R3). envoy-go mirrors this internal naming exactly (the differential `StatsAsserter` + the Prometheus arm depend on it).

### 7.2 The fixed 15-name roster + the EAGER per-command roster (AMEND-R3/R6/R7)

**Fixed roster under `redis.<sp>.` (15 names; created EAGER at config parse):**

| Family | Count | Kind | Created | Incremented |
|---|---|---|---|---|
| `downstream_cx_total`/`downstream_cx_drain_close`/`downstream_cx_protocol_error`/`downstream_cx_rx_bytes_total`/`downstream_cx_tx_bytes_total`/`downstream_rq_total` | 6 | COUNTER | 32.1 | 32.1 (cx/rq for the round-trip) / 32.2 (drain/protocol_error) |
| `downstream_cx_active`/`downstream_cx_rx_bytes_buffered`/`downstream_cx_tx_bytes_buffered`/`downstream_rq_active` | 4 | GAUGE (Accumulate) | 32.1 | 32.2 (the project's 2nd mirrored gauge family) |
| `splitter.invalid_request`/`splitter.unsupported_command` | 2 | COUNTER | 32.2 | 32.2 |
| `upstream_cx_drained`/`max_upstream_unknown_connections_reached`/`connection_rate_limited` (REDIS_CLUSTER_STATS) | 3 | COUNTER | 32.2 | 32.2 (mostly exist-at-0 for the MVP) |
| **Total fixed** | **15** | (11 counters + 4 gauges) | | |

**EAGER per-command roster (32.2; AMEND-R3):** `redis.<sp>.command.<cmd>.{total,success,error}` over the static supported-commands table (~180 commands; the EXACT table + count is the 32.2 IMPL pin, D-P32-2; the live probe enumerated 180 names — §11.2). EAGER creation at config parse (the kafka-176 precedent; `NewCounterIfAbsent` idempotent across listeners sharing a `stat_prefix`), giving roster-present-at-0 parity. DEFERRED per command: the `latency` HISTOGRAM (ADR-0060) + the `delay_fault`/`error_fault` counters (defer with `faults`).

**Upstream traffic stats (AMEND-R6):** `upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total`/… are the cluster's own traffic stats under `cluster.<name>.*`, NOT a redis roster — they come from the existing per-cluster roster the seam reuses (`internal/cluster/manager.go:97-108`). The HTTP-shaped `upstream_rq_2xx..5xx` have no RESP analog (per-side, §7.5). Which upstream/pool counters are differentially-MIRRORED vs pinned per-side is D-P32-2 (resolved at the 32.2 SPEC against a working backend harness — the `0055`/`0056` fixtures are the vehicle).

### 7.3 PING/AUTH local-reply stat accounting (AMEND-R5)

A PING/AUTH connection increments `downstream_cx_total` + `downstream_rq_total` but emits NO `command.*` counter (PING/AUTH are NOT in the command roster) and NO upstream cx/rq (answered locally). The `0055` fixture's PING arm asserts: downstream cx/rq +1, upstream cx/rq 0, the `+PONG\r\n` reply bytes (the downstream-response byte-equivalence prong). The proxied GET/SET arms assert downstream cx/rq +1, `cluster.<name>.upstream_cx_total`/`upstream_rq_total` +1, the proxied reply bytes, and the `command.<cmd>.total/success` increments.

### 7.4 Prometheus exposition — the `redis.` TAG-EXTRACTOR arm (AMEND-R4)

Reference Envoy `contrib-v1.37.2` `/stats/prometheus` (probed live): redis stats emit as **`envoy_redis_<leaf>{envoy_redis_prefix="<stat_prefix>"} <v>`** — the metric name is flat (`envoy_redis_downstream_cx_total`, `envoy_redis_command_get_total`), the stat_prefix is hoisted into the `envoy_redis_prefix` label, every metric family has a `# TYPE` line. This is upstream tag extraction (the `envoy.redis_prefix` well-known tag) — the **mongo `.rbac.` TAG-EXTRACTOR shape (ADR-0218)**, NOT the kafka INLINE shape. The `internal/stats/name.go` `redis.` arm (32.2): detect `redis.<prefix>.<rest>` (leading literal `redis.` + a dot-free `<prefix>` segment) → metric name `envoy_redis_` + `<rest>` flattened (dot→underscore) + label `envoy_redis_prefix="<prefix>"` (the dynamic per-command names flatten identically: `command.get.total` → `envoy_redis_command_get_total{envoy_redis_prefix="…"}`). Shape-based detection (dot-free prefix segment) — an allowlist is impossible given the dynamic command names (the mongo D-P3 precedent). The exact form is pinned by the §11.2 live probe.

### 7.5 Project stat-count delta + departure flags

The fixed 15 land at 32.1 (10: downstream cx/rq) + 32.2 (5: splitter + REDIS_CLUSTER_STATS). The EAGER per-command roster (~180 × 3 = ~540) lands at 32.2 — **the project's largest single-phase stat jump** (a bounded fixed roster from the static command table, the kafka precedent). The exact count + whether the FULL ~180-command table is adopted (vs a core subset) is the 32.2 SPEC pin (D-P32-2 — anticipated FULL eager for roster parity, the kafka-176 precedent). Anticipated: stat surface 536 → 536 + 15 + (per-command roster ~540) ≈ ~1091 at family-row-done (exact at the 32.2 IMPL). The `command.<cmd>.latency` histograms + `*_fault` counters are NOT counted (deferred). Departures (BEHAVIOR_CONTRACT at each IMPL): the latency-HISTOGRAM family unmirrored (ADR-0060); the deferred-active-feature stats (sharding/redirection/fragmentation/AUTH-enforcement/faults/mirroring); `enable_command_stats` no-op-for-counter-presence; the HTTP-shaped `upstream_rq_2xx..5xx` cluster stats with no RESP analog (per-side); runtime-keys-at-defaults; the eager-vs-per-connection creation boot-window difference (unobservable to the differential — all fixture assertions post-connection).

---

## 8. Differential fixture taxonomy (+2)

Full cross-side against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2`. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE directories. Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Numbering continues from `0054` (master-tip tail `0054-kafka-boot-reject`); re-pinned at each sub-phase IMPL Task 1.

**The §9 FIRST — downstream-RESPONSE byte-equivalence as a load-bearing prong.** Unlike the sniffer rows (where bytes pass through unchanged and the stat comparison IS the proof), redis_proxy GENERATES the downstream response by proxying — so the differential proof is TWO-pronged: (1) the RESP bytes returned to the client are byte-identical cross-side (`reference_wire_format_both_sides_see_same_bytes`, EXTENDED to the response the proxy generates); (2) cross-side `StatsAsserter` over the `redis.<sp>.` roster. `upstream_cx_*`/pooling stats that depend on connection-reuse internals are pinned per-side where the reference diverges (`reference_close_direction_framework_gap` / `reference_differential_reference_parses_full_message` precedent — pin per-side values, not equality).

**Fixture-design caveats from the §11.2 live probe:** (i) the fixture backend MUST be a real listening TCP RESP responder reachable via a STRICT_DNS hostname on a shared bridge network (`reference_docker_probe_bridge_network`); verify the round-trip ran via `cluster.<name>.upstream_rq_total > 0` + a non-empty downstream RESP reply; (ii) drive RESP2 (RESP3 `HELLO 3` is rejected `NOPROTO`); (iii) PING/AUTH arms expect ZERO upstream traffic (AMEND-R5).

### 8.1 `0055-redis-roundtrip` (32.1 + 32.2; cross-side; multi-arm)

Chain `[redis_proxy]` as the TERMINAL on BOTH sides (the contrib reference Envoy + envoy-go; NO `tcp_proxy` — redis_proxy terminates), `catch_all_route.cluster` → the new `TCPRedisResponder` backend (§8.3); the driver sends real RESP commands as a Redis client. Arms (the 32.x SPECs finalize):

1. **PING (local-reply; 32.1)** — `PING\r\n` (inline) + `*1\r\n$4\r\nPING\r\n` (array) → `+PONG\r\n` reply byte-equivalence; `downstream_rq_total` +1; upstream cx/rq 0 (AMEND-R5).
2. **proxied round-trip (32.1)** — `*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n` → `+OK\r\n`; `*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n` → `$3\r\nbar\r\n` → downstream-RESP byte-equivalence + `cluster.<name>.upstream_rq_total` +1 + (32.2) `command.set.total`/`command.get.total` +1.
3. **command matrix (32.2)** — GET-miss → `$-1\r\n` null bulk (counts `command.get.success`, not error); INCR → `:1\r\n`; a command spread; an AUTH-no-password arm → `-ERR Client sent AUTH, but no password is set\r\n` (local; upstream 0).
4. **splitter arms (32.2)** — an unsupported command (`UNKNOWN`) → `-ERR unknown command 'UNKNOWN', …\r\n` + `splitter.unsupported_command` +1; a bad-arity command (`SELECT` with wrong arity) → `-invalid request\r\n` + `splitter.invalid_request` +1.
5. **deliberate-break liveness proof** — recorded in driver comments + README per the `0030` lesson, run with `-count=1` (the cross-side `StatsAsserter` + the downstream-response byte-equivalence are the load-bearing proofs — prove each asserted counter + each response-byte assertion LIVE).

(Per the cross-side-XOR-boot-reject constraint, all cross-side arms share this ONE dir; 32.1 lands the PING + proxied round-trip arms, 32.2 extends with the command matrix + splitter arms.)

### 8.2 `0056-redis-boot-reject` (32.1; boot-reject; separate dir)

Missing `stat_prefix` → both sides reject at boot (the §6.2 `redis-proxy-stat-prefix-required` arm; boot-stderr-substring parity per §6.1). The `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` arms are unit-tested; whether any gain a fixture arm here is D-P32-5 (anticipated: `0056` carries the `stat_prefix` arm only — the kafka `0054`/mongo `0050` precedent).

### 8.3 New BackendKind (anticipated value 32)

A NEW BackendKind — a synthesized **`TCPRedisResponder`** speaking minimal RESP: it reads RESP request frames and returns canned replies for the exercised commands (`+OK` for SET, `$<n>\r\n<val>\r\n` bulk for GET-hit, `$-1\r\n` for GET-miss, `:<n>` for INCR). FIFO/positional — NO correlation id (contrast the kafka `TCPKafkaResponder` (31) correlation-id-echoing; the mongo `TCPMongoResponder` (30); the silent `TCPSink` (28)). Note PING/AUTH never reach the backend (local-reply, AMEND-R5) — the responder need not handle them. The exact canned-reply table is pinned at the 32.1 IMPL.

### 8.4 Total fixture-dir count + conformance

56 → **58** (+2: `0055` cross-side at 32.1 [extended at 32.2], `0056` boot-reject at 32.1; 57 if the boot-reject arm folds — D-P32-5). No new conformance harness (matches 26/27/28/29/31). The h2spec 53/53 + proxy-wasm 10/10 gates re-run asserted-unaffected at each sub-phase six-gate (image-independent; phase 32 touches no HTTP/h2/proxy-wasm path).

---

## 9. Behavior-contract delta (per ADR-0052 atomic landing)

BEHAVIOR_CONTRACT.md gains phase-32 content in two passes (one bundle per sub-phase IMPL final task):

- **32.1 bundle**: NEW `### envoy.filters.network.redis_proxy` subsection (the terminal-routing-proxy semantics; the RESP codec value-type framing + inline + null sentinels; the `catch_all` single-cluster routing; the PING/AUTH local-reply set; the config parse + the PGV/runtime reject arms; the downstream cx/rq counters) + a NEW `## Network filters — upstream connection-pool / cluster-routing seam (32.1)` framework subsection (the seam's API + the FIFO/positional correlation contract + the redis-scoped boundary); stat table 536 → 536 + 10.
- **32.2 bundle**: the full command-set + stat-roster extension of the redis_proxy subsection (the EAGER per-command roster; the splitter + REDIS_CLUSTER_STATS counters; the 4 gauges' inc/dec; the `redis.` Prometheus tag-extractor flattening; the downstream-response byte-equivalence proof); the coverage-boundary / departure records (the `command.<cmd>.latency` histograms unmirrored ADR-0060; the deferred active features; `enable_command_stats` no-op; the HTTP-shaped upstream_rq_2xx..5xx per-side; runtime-keys-at-defaults); stat table 536 + 10 → 536 + 10 + 5 + per-command roster; the parent-row-32 family rollup note.

---

## 10. ADR anchor map (1 §Context draft at THIS parent-SPEC commit)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL) + the BRAINSTORM §7 locked numbering. At THIS parent-SPEC commit, the **ADR-0229 §Context draft** is appended to DECISIONS.md (tail ADR-0228 → ADR-0229; next-free → ADR-0230). UNLIKE the phase-28/29 precedent of drafting all sub-phase ADRs at the parent SPEC, **ADR-0230 (the upstream-pool seam) anchors its §Context at the 32.1 SPEC** (its exact API is a 32.1-SPEC pin — D-P32-3 — so the §Context is authored where the design lands; the BRAINSTORM §7 + next-prompt §11 specify this). No ADR number beyond ADR-0229's §Context is consumed at this session.

- **ADR-0229** *(32 — filter + parent umbrella; §Context here, §Decision/§Consequences at the 32.1/32.2 IMPL)* — the `redis_proxy` filter: the single-route-terminal envelope (RESP codec + `catch_all` single-cluster routing + the core single-key command set + PING/AUTH local-reply + the fixed 15-name roster + the EAGER per-command roster under `redis.<stat_prefix>.`), the `redis.` Prometheus TAG-EXTRACTOR arm, the 10th built-in, the deferred-active-feature posture (parse-accept per field; sharding/redirection/fragmentation/AUTH-enforcement/faults/mirroring deferred), the cross-side differential (downstream-RESP byte-equivalence + stat parity), the `TCPRedisResponder` BackendKind, the 41st fuzzer `FuzzRESPDecode`, and ZERO new go.mod dep (core `/envoy`). The parent-row umbrella ADR for the 2-way split.
- **ADR-0230** *(32.1 — the upstream connection-pool / cluster-routing seam; §Context at the 32.1 SPEC, NOT here; §Decision/§Consequences at the 32.1 IMPL)* — the framework's SIXTH structural extension: a terminal filter resolves a cluster host via `cluster.Manager`, dials via `cluster.Cluster.Dial`, pools, and pipelines request↔reply in FIFO/positional order; redis-scoped (no thrift generalization); builds on `TerminalFilter.Handle`. The seam ADR (the analogue of ADR-0221 [WriteFilter seam] / ADR-0226 [async halt/resume seam]).

Next-free after phase-32 phase-done ≈ **ADR-0231**.

---

## 11. Empirical-pin block (D32-1..D32-8 resolved at this SPEC session)

Parallel-subagent-fan-out scrape executed during this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-08.** The 8 pins span both sub-phases; resolved once here; sub-phase SPECs reference this block.

**Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image** (id `7edd5b0f…`, present locally): a real boot of a `[redis_proxy (stat_prefix: redisprobe)]` terminal listener on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with a real `redis:7` backend (STRICT_DNS hostname `redisbackend:6379`); driven RESP traffic (PING/SET/GET/INCR/AUTH/UNKNOWN/bad-arity) via `redis-cli` + raw TCP; admin `/stats` + `/stats/prometheus` scrapes pre- and post-connection; `--mode validate` boot-reject probes. **Round-trip CONFIRMED ran:** `cluster.redis_cluster.upstream_rq_total` 0 → 5; real replies `+PONG`/`+OK`/`$3\r\nbar`/`:1`/`$-1`.
2. **go-control-plane `/envoy` v1.32.4 bindings** at `~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/redis_proxy/v3/`: `redis_proxy.pb.go` + `.pb.validate.go` + `_vtproto.pb.go`; `proto.MessageName` run in a throwaway module + the go.mod dep-closure check.
3. **Upstream Envoy v1.37.2 source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/filters/network/redis_proxy/{proxy_filter,command_splitter_impl,conn_pool_impl,router_impl,config}.{h,cc}`; `source/extensions/filters/network/common/redis/{codec_impl,client_impl}.{h,cc}` + `supported_commands.h`.
4. **envoy-go codebase** at the worktree tip `2fed159`: `internal/filter/network/{terminal,types,upstreamcluster}.go` + `tcpproxy/filter.go` + `cluster/manager.go` + `stats/name.go` + `bootstrap/bootstrap.go` + `builtins/builtins.go`.

### Summary disposition table (8 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D32-1 (SPEC-BLOCKING) — TypeURL + dep | **CONFIRMED** (@type carries `extensions.`; ZERO new go.mod dep — core `/envoy`) | R1 |
| §11.2 | D32-2 (SPEC-BLOCKING) — stat roster + creation + prom form | **RESOLVES** (15 fixed [11c+4g] + EAGER ~180-cmd table; LABEL-HOISTED prom) + REFUTES (eager-not-dynamic; enable_command_stats no-op) | R3, R4, R6, R7 |
| §11.3 | D32-3 (SPEC-BLOCKING) — the seam model | **RESOLVES** (one-client-per-host multiplexed; FIFO positional pop-front; catch_all→cluster→host→dial) | R8 |
| §11.4 | D32-4 — config PGV + parse-reject | **RESOLVES** (settings + op_timeout PGV-required; runtime missing-upstream reject; unknown-cluster tolerated; deferred fields parse-accept) | R2 |
| §11.5 | D32-5 — RESP framing + response bytes | **RESOLVES** (value-type framing verbatim; per-command reply bytes captured) | — |
| §11.6 | D32-6 (SPEC-BLOCKING) — PING/AUTH local-reply | **RESOLVES** (PING/AUTH/ECHO/TIME/QUIT/HELLO local; zero upstream; count downstream_rq only) | R5 |
| §11.7 | D32-7 — IsValidName placement | **RESOLVES** (per-command segment wire-derived BUT table-bounded eager → guard satisfied by construction if the eager-table posture is adopted; codec-boundary guard if dynamic — D-P32-7) | R3 |
| §11.8 | D32-8 — close-direction + LoC envelope | **RESOLVES** (NO close-direction counters; ~1100-1550 prod LoC → 2-way split holds) | R9 |

### 11.1 D32-1 (SPEC-BLOCKING) — TypeURL + the dep: CONFIRMED

`proto.MessageName(&redis_proxyv3.RedisProxy{})` (run in a throwaway module) = `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` → `@type` = `type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` (carries the `extensions.` segment — `reference_network_filter_typeurl_extensions` holds). The redis_proxy/v3 package imports only `config/core/v3` + `extensions/common/dynamic_forward_proxy/v3` (both inside the ALREADY-DIRECT `/envoy v1.32.4` module) + `cncf/xds/go` + `protoc-gen-validate` + `google.golang.org/protobuf` (all already in the envoy-go closure). **Importing redis_proxy/v3 adds ZERO new go.mod module requirements** — the exact opposite of kafka_broker's first-`/contrib` dep; `go mod tidy` adds nothing. The IMPL Task-1 pinning test confirms `proto.MessageName` + a clean `go mod tidy`. No nested `*PerRoute` message.

### 11.2 D32-2 (SPEC-BLOCKING) — stat roster + creation timing + Prometheus form

**Live-decode achieved** (round-trip ran). Pins:

- **Scope:** `redis.<stat_prefix>.` (live: `redis.redisprobe.*`).
- **Fixed roster (AMEND-R7):** `ALL_REDIS_PROXY_STATS` = 6 counters (`downstream_cx_total`/`downstream_cx_drain_close`/`downstream_cx_protocol_error`/`downstream_cx_rx_bytes_total`/`downstream_cx_tx_bytes_total`/`downstream_rq_total`) + 4 gauges Accumulate (`downstream_cx_active`/`downstream_cx_rx_bytes_buffered`/`downstream_cx_tx_bytes_buffered`/`downstream_rq_active`); + 2 splitter counters (`splitter.invalid_request`/`splitter.unsupported_command`); + 3 REDIS_CLUSTER_STATS (`upstream_cx_drained`/`max_upstream_unknown_connections_reached`/`connection_rate_limited`) = 15 names under `redis.<sp>.`. Observed live: `downstream_cx_total=8`, `downstream_rq_total=8`, `downstream_cx_rx_bytes_total=189`, `splitter.invalid_request=1`, `splitter.unsupported_command=2`.
- **Per-command roster (AMEND-R3 — EAGER, REFUTES dynamic):** ALL ~180 splitter-registered commands present-at-0 from boot (1080 `command.*` lines = ~180 × {total/success/error counters + latency histogram + delay_fault/error_fault counters}), identical before/after traffic AND identical with `enable_command_stats` on/off. Source: `command_splitter_impl.cc addHandler()` pre-creates eagerly over the static `SupportedCommands` table. `command.<cmd>` for a wire command NOT in the table is never built (→ `splitter.unsupported_command`). The 180-name list is transcribed for the 32.2 IMPL table (D-P32-2).
- **Upstream stats (AMEND-R6):** `upstream_cx_*`/`upstream_rq_*` are the cluster's own traffic stats under `cluster.<name>.*` (live: `cluster.redis_cluster.upstream_cx_total`/`upstream_rq_total`), NOT a redis roster. Only the 3 REDIS_CLUSTER_STATS are redis-specific.
- **Prometheus (AMEND-R4 — LABEL-HOISTED):** `envoy_redis_<leaf>{envoy_redis_prefix="<sp>"}` (live: `envoy_redis_command_get_total{envoy_redis_prefix="redisprobe"} 2`; `envoy_redis_downstream_cx_total{envoy_redis_prefix="redisprobe"} 8`). The mongo `.rbac.` tag-extractor shape, NOT the kafka inline shape.

**PROBE-HARNESS CAVEAT (recorded honestly):** `enable_command_stats` toggled NEITHER presence NOR increment in contrib-v1.37.2 (on vs off byte-identical) — what (if anything) the flag still gates is not observable from the stat roster; treat per-command stats as always-on for fixture purposes. The DYNAMIC-vs-eager + full-180-vs-subset adoption decision + the exact mirrored-vs-per-side upstream counter set are 32.2-SPEC re-pins (D-P32-2) — the `0055` fixture (a working backend harness) is the vehicle.

### 11.3 D32-3 (SPEC-BLOCKING) — the upstream connection-pool / cluster-routing + reply-correlation model

The model envoy-go's seam (ADR-0230) mirrors (source):

- **Routing:** `router_impl.cc RouterImpl::upstreamPool()` — longest-prefix-match of the command KEY against the route table; on miss → `catch_all_route_`; no catch_all + no match → nullptr (rejected). MVP: catch_all → single cluster.
- **Cluster → host:** `conn_pool_impl.cc ThreadLocalPool::makeRequest()` — `cluster_->loadBalancer().chooseHost(lb_context) → host`. envoy-go's `Cluster.Dial` already encapsulates round-robin host selection → the seam reuses it.
- **Pool model:** `conn_pool_impl.h` `client_map_[host] → one ThreadLocalActiveClient` (ONE upstream connection per host, shared across ALL downstream conns, thread-local); each `ClientImpl` wraps exactly one network connection. Multiplexing: replies route back to the right downstream via the per-request `ClientCallbacks`, NOT connection identity.
- **Reply correlation (the load-bearing invariant):** RESP has no correlation id → strict request order. `client_impl.h` `std::list<PendingRequest> pending_requests_` (FIFO): send `emplace_back` + `upstream_rq_active_.inc()`; reply (`onRespValue`) `pending_requests_.front()` + `pop_front()` (positional); canceled requests stay in the queue flagged + are popped+discarded in order so positions stay aligned.
- **envoy-go MVP recommendation:** `resolve catch_all cluster via cluster.Manager.Get → Cluster.Dial → pool per host (OR the simplest one-conn-per-downstream model — no cross-downstream multiplexing, trivially-correct positional correlation) → FIFO pending-request queue → positional pop-front reply correlation`. The FIFO+positional contract is load-bearing either way; the multiplexing model is the 32.1-SPEC D-P32-3.

### 11.4 D32-4 — config PGV arms + parse-reject

§5/§6 transcribe the full roster + arms. Live `--mode validate` against the contrib image: THREE PGV hard rejects (`stat_prefix` min-1; `settings` required; `settings.op_timeout` required) + ONE runtime reject (`cannot configure a redis-proxy without any upstream` — neither catch_all nor routes supplies an upstream; same string for `prefix_routes` omitted AND `prefix_routes: {}`). Unknown cluster refs (catch_all `cluster`, external_auth `grpc_service`) TOLERATED at validate. All deferred fields (faults, downstream_auth_password, external_auth_provider, enable_redirection-without-dns-cache) parse-accept standalone. The `catch_all_route.cluster` is PGV-required min-1 when a catch_all is present. The C++ wording prefix `Proto constraint validation failed (...)` for PGV rejects; the runtime reject has NO such prefix.

### 11.5 D32-5 — RESP wire framing + downstream response bytes

RESP value-type framing (source `codec_impl.cc` + live capture): simple-string `+...\r\n`, error `-...\r\n`, integer `:<n>\r\n`, bulk-string `$<len>\r\n<bytes>\r\n`, null bulk `$-1\r\n`, array `*<n>\r\n...`, null array `*-1\r\n`; inline commands accepted (`PING\r\n` parsed as a space-separated token array). Request array shape `*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n`. Per-command reply bytes captured verbatim (live): `PING`→`+PONG\r\n`; `SET foo bar`→`+OK\r\n`; `GET foo`→`$3\r\nbar\r\n`; `GET noexist`→`$-1\r\n` (counts `success`, not error); `INCR ctr`→`:1\r\n`; `AUTH foo` (no auth)→`-ERR Client sent AUTH, but no password is set\r\n`; `UNKNOWN`→`-ERR unknown command 'UNKNOWN', with args beginning with: \r\n`; bad-arity `SELECT`→`-invalid request\r\n`. The downstream RESPONSE bytes are GENERATED by redis_proxy (proxied from upstream for data commands; locally for PING/AUTH) — these are the byte-equivalence-prong reference bytes (§8). Adopt the framing VERBATIM (`reference_wire_format_both_sides_see_same_bytes`, extended to the response).

### 11.6 D32-6 (SPEC-BLOCKING) — PING/AUTH local-reply semantics

PING answered LOCALLY (`+PONG\r\n`), NOT forwarded — a PING-only connection left `cluster.redis_cluster.upstream_cx_total`/`upstream_rq_total` at 0 while `redis.<sp>.downstream_rq_total` incremented; `command.ping.*` does not exist (PING outside command-stat accounting); PING-with-arg still returns `+PONG\r\n` (arg ignored — does NOT echo a real redis server's behavior). AUTH (no `downstream_auth_password` configured) answered locally with `-ERR Client sent AUTH, but no password is set\r\n`, zero upstream. Source: `command_splitter_impl.cc` + `proxy_filter.cc onAuth()` handle PING/AUTH/ECHO/TIME/QUIT/HELLO in-filter; only data commands proxy via `makeRequest()`. The MVP minimal local-reply set is PING + AUTH (ECHO/TIME/QUIT/HELLO are easy 32.2 follow-ons — D-P32-6). Fixture consequence: PING/AUTH arms expect ZERO upstream traffic.

### 11.7 D32-7 — IsValidName / dynamic-stat-name charset

The per-command stat segment is `absl::AsciiStrToLower(wire-command-name)` — wire-derived (a redis client can send any byte string as a command). Upstream is protected by EAGER creation over the STATIC `SupportedCommands` table (a wire command not in the table never builds a per-command counter → routes to `splitter.unsupported_command`) — so no raw wire bytes reach a stat-name builder. **Implication:** if envoy-go replicates the eager-static-table posture (AMEND-R3 — the kafka precedent), the `IsValidName` guard is satisfied BY CONSTRUCTION (table-bounded names, like kafka); if envoy-go instead builds `command.<cmd>` lazily from the wire via `NewCounterIfAbsent`, the guard is LOAD-BEARING (`reference_dynamic_stat_name_charset_guard` — `NewCounterIfAbsent` PANICS on invalid names; the mongo precedent). The SPEC's anticipated posture is the EAGER static table (roster parity + guard-by-construction); the exact disposition (eager-table vs dynamic-with-guard) is the 32.2-SPEC D-P32-7. Either way the `FuzzRESPDecode` no-panic fuzzer asserts the decode path never panics + never mutates the chain buffer.

### 11.8 D32-8 — close-direction counters + LoC envelope

**Close-direction counters: NONE.** `ALL_REDIS_PROXY_STATS` has no `cx_destroy_local/remote`-style direction-keyed counter (contrast mongo's `cx_destroy_local/remote_with_active_rq`) — only `downstream_cx_drain_close` (a drain counter, gated on the drain decision) + `upstream_cx_drained`. redis_proxy stays close-direction-framework-zero-touch (the 29.3 `CloseDirection` machinery is not needed; `reference_close_direction_framework_gap` does NOT bite).

**LoC envelope (rough; for the 2-way split vs ADR-0045 ~25 tasks / ~1500 prod LoC/phase):** RESP codec (encode + decode state machine; 6 value types + inline + null sentinels + nested-array stack) ~350-450; the upstream-pool seam (catch_all resolve → dial → pool → FIFO positional correlation; the NEW seam) ~300-450; the redisproxy filter glue (TerminalFilter.Handle pump + decoder + downstream_cx lifecycle + PING/AUTH local replies + splitter dispatch) ~300-400; stats (15 fixed + the eager per-command roster + the IsValidName disposition) ~150-250. Total ~1100-1550 production LoC. **The 2-way split holds:** 32.1 = seam + codec + round-trip (~codec 400 + seam 400 + minimal glue 250 + downstream_cx stats 120 ≈ 1170); 32.2 = the full command set + stats + the differential matrix. Each side lands under the gate (the PLAN re-checks).

---

## 12. SPEC-time D-questions for sub-phase SPEC / PLAN resolution

- **D-P32-1 (fixed-roster creation posture).** Eager-at-config-parse (freeze-after-boot friendly; boot-window departure recorded) vs first-connection (upstream parity). **Resolution at:** 32.1 SPEC. Anticipated: eager-at-config-parse (the kafka/mongo precedent); all fixture assertions post-connection make the difference unobservable.
- **D-P32-2 (per-command roster: eager-full vs subset + the exact ~180-cmd table + the mirrored-vs-per-side upstream counter set).** §7.2 anticipates the FULL eager ~180-command table (roster parity, the kafka-176 precedent) over `command.<cmd>.{total,success,error}`. The exact table (transcribe the static SupportedCommands sets from `supported_commands.h`) + whether the full table or a core subset is adopted + which `upstream_cx_*`/`cluster.<name>.*` counters are differentially-mirrored vs pinned per-side. **Resolution at:** 32.2 SPEC (re-probe via the `0055` working-backend harness).
- **D-P32-3 (the seam API + multiplexing model).** The exact exported type(s) the redisproxy terminal calls; one-upstream-conn-per-downstream-conn (the simplest faithful MVP) vs a shared per-host pool with a per-conn pending queue; the FIFO/positional pop-front contract. **Resolution at:** 32.1 SPEC (the ADR-0230 §Context anchors here).
- **D-P32-4 (RESP codec partial-frame reassembly model).** The decoder-internal buffering for partial frames across reads (the mongo/kafka private-buffer precedent, adapted to a TERMINAL filter that owns the conn — no chain Buffer). **Resolution at:** 32.1 SPEC.
- **D-P32-5 (boot-reject fixture arms).** Do the `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` reject arms gain `0056` fixture arms or stay unit-test-only? **Resolution at:** 32.1 SPEC (anticipated: `0056` carries the `stat_prefix` arm; the rest unit-tested — the kafka `0054`/mongo `0050` precedent).
- **D-P32-6 (local-reply command set extent).** The MVP minimal set is PING + AUTH (32.1); ECHO/TIME/QUIT/HELLO are 32.2 follow-ons. **Resolution at:** 32.2 SPEC.
- **D-P32-7 (IsValidName disposition).** Eager-static-table (guard satisfied by construction, the kafka posture) vs dynamic-with-`IsValidName`-guard (the mongo posture). **Resolution at:** 32.2 SPEC. Anticipated: eager-static-table.
- **D-P32-8 (the 4 downstream gauges' differential assertion design).** How `0055` asserts the gauges deterministically at quiesced points (the mongo `op_query_active` D-P9/D-P10 precedent — the project's first mirrored gauge). **Resolution at:** 32.2 SPEC.
- **D-P32-9 (upstream-pool stat per-side asymmetry).** The reference's pooling internals (one-client-per-host multiplexed) diverge from envoy-go's MVP (one-conn-per-downstream) → `upstream_cx_*` counts differ; pin per-side (the `reference_close_direction_framework_gap` / `reference_differential_reference_parses_full_message` precedent). **Resolution at:** 32.2 SPEC.

---

## 13. RATIFIED-PENDING-IMPL items

- **R1 (seam back-compat).** The upstream-pool seam is ADDITIVE (it activates only for a redis_proxy terminal); all existing fixtures (`0000`..`0054`) stay byte-exact green at 32.1 + 32.2 (the seam's regression gate). `TerminalFilter.Handle`/`tcp_proxy`/HCM untouched.
- **R2 (roster + scope parity).** The fixed 15-name roster + the EAGER per-command roster under `redis.<stat_prefix>.` match upstream name-for-name. Ratified by the `0055` post-connection roster assertion + a `TestStatRoster_MatchesUpstream` + `TestCommandRoster_MatchesUpstream` byte-stable test.
- **R3 (downstream-response byte-equivalence — the §9 FIRST).** The RESP bytes redis_proxy returns to the client are byte-identical cross-side for every `0055` arm (PING `+PONG`, SET `+OK`, GET-hit bulk, GET-miss `$-1`, INCR `:n`, AUTH error, unsupported/bad-arity errors). Ratified by the `0055` response-byte assertion + the deliberate-break liveness proof.
- **R4 (FIFO/positional correlation).** Replies dequeue the oldest pending request positionally (pop-front); the single-route single-key envelope keeps the queue trivially in-order. Ratified by the `0055` multi-command round-trip + a unit test.
- **R5 (PING/AUTH local-reply; zero upstream).** PING/AUTH are answered in-filter with zero upstream traffic (AMEND-R5); they count `downstream_rq_total` but no `command.*`/upstream. Ratified by the `0055` PING/AUTH arms (upstream cx/rq asserted 0).
- **R6 (StatsAsserter liveness).** Every `0055` stat assertion is proven live via a recorded deliberate-break run with `-count=1` (`reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1`).
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` redis lines match the reference's tag-extracted form `envoy_redis_<leaf>{envoy_redis_prefix="<prefix>"}` (AMEND-R4). Ratified by the `0055` StatsAsserter scrape.
- **R8 (zero new dep).** redis_proxy/v3 is core `/envoy v1.32.4` → `go mod tidy` adds nothing with the first consumer; the @type pinned via `proto.MessageName`. Ratified by the 32.1 IMPL Task-1 pinning test + a clean `go mod tidy`.
- **R9 (counts re-pinned).** Each sub-phase IMPL Task 1 re-pins: fuzzers 40 (→41 at 32.2), fixtures 56 (tail `0054`; →57-58 at 32.1), stat surface 536 (→ +10 at 32.1, → +5 + per-command roster at 32.2), BackendKind tail 31 (→32 at 32.1), DECISIONS.md tail ADR-0229 (next-free ADR-0230; ADR-0230 §Context at the 32.1 SPEC) — against the live IMPL-session tip.

---

## 14. Test surface

Per the §9 family precedent (unit + fuzz + differential + race), per sub-phase:

- **32.1**: Layer A unit tests at `internal/filter/network/redisproxy/` (config parse incl. all PGV + runtime-reject arms; the RESP codec per value type + null sentinels + inline + partial-frame reassembly + malformed-frame handling; the `TerminalFilter.Handle` pump; PING/AUTH local-reply) + `internal/filter/network/` (the upstream-pool seam — host resolve + dial + FIFO positional correlation; a unit/`-race` test for the round-trip); Layer D fixtures `0055` (round-trip arm) + `0056` + the FULL back-compat suite (56 existing dirs); Layer E `-race -short`. Per-task `gofmt -l` + `golangci-lint` on touched pkgs (`feedback_pertask_gofmt_lint`).
- **32.2**: unit tests for the full command set + the eager per-command roster + the splitter/REDIS_CLUSTER_STATS counters + the 4 gauges + the `redis.` tag-extractor arm (incl. dynamic-command-name flattening) + the IsValidName disposition; the 41st fuzzer `FuzzRESPDecode`; fixture `0055` extended (command matrix + splitter arms) + the full suite; race.
- **Six-gate checklist** (per phase-26/27/28/29/31): `go build` / `go vet` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 32 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly).

---

## 15. Per-sub-phase split-gate confirmation (D32-8 → ADR-0045)

| Sub-phase | Production LoC | Tasks | ADR-0045 gate (~25t / ~1500 LoC) | Verdict |
|---|---|---|---|---|
| 32.1 (seam + codec + round-trip) | ~900-1200 | ~14-20 | fits | ✅ (the seam + codec dominate; the PLAN re-checks) |
| 32.2 (commands + stats + matrix) | ~400-650 | ~10-15 | fits | ✅ |

Each sub-phase is independently shippable + delivers value: 32.1 ships a real single-command redis proxy with a live cross-side round-trip + the upstream-pool seam; 32.2 completes the command + stat surface + the differential matrix + the family rollup. The 2-way pre-split holds at parent-SPEC time. A 32.x escape-valve split stays available if either sub-phase trips the gate at its PLAN.

---

## 16. Stage-close handoff

Per ADR-0004/0005 (autonomous adaptation): this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, STATE.md advances to lifecycle-state 2-for-32.1 with `next-skill = superpowers:writing-plans` scoped to the **32.1 SPEC** (per the per-sub-phase precedent — the next session authors the 32.1 sub-phase SPEC, which anchors the ADR-0230 §Context for the upstream-pool seam; NOT the 32.1 PLAN, mirroring 22.1/25.1/26.1/28.1/29.1). ROADMAP: parent row 32 STAYS `in-progress`; sub-rows 32.1/32.2 STAY `planned` (32.1 flips `planned → in-progress` at the **32.1 SPEC** commit, NOT at this parent SPEC — the 26.x/28.x/29.x precedent). The parent SPEC + the ADR-0229 §Context draft are squash-merged to master + pushed (`feedback_push_to_origin`; the controller squash-merges + pushes at stage-close, subagents local-only per `feedback_subagents_no_push`); next-prompt.txt is rewritten for the 32.1-SPEC cold-start.

---

## Appendix A — Phase 32 ADR landing summary

- **ADR-0229** *(§Context drafted at this parent SPEC; §Decision/§Consequences body at the 32.1/32.2 IMPL per ADR-0044)* — the `redis_proxy` filter + the parent-row-32 umbrella: TypeURL via `proto.MessageName` from the CORE `/envoy` module (the `extensions.` segment; ZERO new go.mod dep — UNLIKE kafka_broker); the single-route-terminal envelope (the in-house RESP codec — value types + inline + null sentinels; `catch_all_route` single-cluster routing; the core single-key command set + PING/AUTH local-reply [zero upstream]); the fixed 15-name roster under `redis.<stat_prefix>.` (11 counters + 4 gauges — the project's 2nd mirrored gauge family) + the 2 splitter counters + the 3 REDIS_CLUSTER_STATS + the EAGER per-command `command.<cmd>.{total,success,error}` roster over the static ~180-command table (the kafka-eager precedent — REFUTING the BRAINSTORM dynamic hypothesis; the `command.<cmd>.latency` histograms + `*_fault` counters deferred); the `redis.` Prometheus TAG-EXTRACTOR arm (`envoy_redis_<leaf>{envoy_redis_prefix="<prefix>"}` — the mongo shape, NOT the kafka inline shape); the deferred-active-feature posture (prefix_routes-multi-cluster / sharding+redirection / multi-key-fragmentation / AUTH-enforcement / faults / mirroring / read_policy all parse-accepted-behavior-deferred; `settings` + `settings.op_timeout` PGV-required; the runtime missing-upstream reject); the cross-side differential (downstream-RESP-response byte-equivalence — the §9 FIRST — PLUS cross-side `StatsAsserter`; `0055`/`0056`) + the `TCPRedisResponder` BackendKind (FIFO/positional) + the 41st fuzzer `FuzzRESPDecode`; the 10th built-in.
- **ADR-0230** *(§Context drafted at the 32.1 SPEC — NOT here; §Decision/§Consequences body at the 32.1 IMPL)* — the upstream connection-pool / cluster-routing seam: the framework's SIXTH structural extension, in the existing `internal/filter/network/` package — a terminal filter resolves a host of a named cluster via `cluster.Manager.Get`, dials via `cluster.Cluster.Dial` (the tcp_proxy dial path), pools the conn per upstream host, and pipelines request↔reply in FIFO/positional order (RESP carries no correlation id); redis-scoped (no thrift generalization; YAGNI — thrift reuses/extends it); builds on `TerminalFilter.Handle` (UNCHANGED); additive (existing fixtures byte-identical). The seam ADR (the analogue of ADR-0221 [WriteFilter seam] / ADR-0226 [async halt/resume seam]).

Next-free after phase-32 phase-done ≈ **ADR-0231**.
