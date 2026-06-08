# Phase 32.1 SPEC — the upstream connection-pool / cluster-routing seam + the RESP codec + the redisproxy round-trip foundation

> **For agentic workers:** this is the per-sub-phase SPEC for **phase 32.1** (`network-filter-redis-upstream-pool-seam-and-codec`), the FIRST sub-phase of the phase-32 BRAINSTORM-time 2-way pre-split (32.1 / 32.2). It is authored per the phase-22.1 / 25.1 / 26.1 / 28.1 / 29.1 per-sub-phase-SPEC precedent: the **parent SPEC** (`docs/envoy-go/phases/32-network-filter-redis-proxy/SPEC.md`) already resolved the BRAINSTORM §10 D32-1..D32-8 empirical pins IN-SESSION live against the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` + go-control-plane `/envoy` v1.32.4 + upstream Envoy v1.37.2 source (parent §11; AMEND-R1..R9), formalized the 2-way split surface-mapping (parent §3), and anchored the ADR-0229 §Context. This 32.1 SPEC **INHERITS** the parent SPEC's §5 proto roster + §6 PARSE-REJECT roster + §7 stat surface + §8 fixture taxonomy + §11 empirical-pin block + §13 RATIFIED-PENDING items, **resolves the parent's 32.1-owned D-questions** (D-P32-1 creation posture, D-P32-3 the seam API + multiplexing model, D-P32-4 the RESP partial-frame reassembly model, D-P32-5 boot-reject fixture arms, D-P32-6 the 32.1 local-reply subset), and **anchors the ADR-0230 §Context** (the upstream connection-pool / cluster-routing seam, deferred from the parent SPEC per the BRAINSTORM §7). It runs NO new docker probes (the parent §11 D32-1..D32-8 block is the authoritative empirical record — referenced, not re-executed). The next session, per BOOTSTRAP §5, authors the **32.1 PLAN** (bite-sized TDD tasks) from this SPEC.

**Goal:** Land a real single-command `redis_proxy` terminal proxy with a live cross-side round-trip — the project's FIRST terminal routing proxy — built on (a) the NEW **upstream connection-pool / cluster-routing seam** (the framework's SIXTH structural extension, ADR-0230, in `internal/filter/network/`), (b) the NEW `internal/filter/network/redisproxy/` package (TypeURL + config parse + the in-house RESP codec + the `TerminalFilter.Handle` command→reply pump), (c) the 10th built-in + the `redis_proxy/v3` blank-import (ZERO new go.mod dep), (d) the `TCPRedisResponder` BackendKind, and (e) fixtures `0055-redis-roundtrip` (cross-side; PING local-reply + a proxied SET/GET round-trip) + `0056-redis-boot-reject`.

**Architecture:** A NEW `internal/filter/network/redisproxy/` package implements `network.TerminalFilter` (NOT `ReadFilter`/`WriteFilter` — it TERMINATES the downstream connection via `Handle(ctx, conn)`; there is no `tcp_proxy` behind it). Per accepted downstream connection, `Handle` owns the raw `net.Conn`, reads RESP request frames from a `bufio.Reader` over it (partial frames simply block — the terminal owns the conn, UNLIKE the mongo/kafka observer private-buffer model), dispatches each request: PING/AUTH are answered LOCALLY (zero upstream; AMEND-R5), data commands round-trip through the **upstream-pool seam** — one upstream connection per downstream connection (the simplest faithful MVP; AMEND-R8), lazily dialed on the first proxied command over the as-built `cluster.Cluster.Dial` path, with FIFO/positional reply correlation (a synchronous single-flight pump — RESP carries no correlation id). The reply bytes are forwarded VERBATIM downstream (`reference_wire_format_both_sides_see_same_bytes`, EXTENDED to the downstream RESPONSE the proxy generates). The differential proof is TWO-pronged and a §9 FIRST: downstream-RESP-RESPONSE byte-equivalence PLUS cross-side `StatsAsserter` parity over the flat admin `/stats` `redis.<stat_prefix>.` names.

**Tech Stack:** Go 1.26.2; golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy` v1.32.4 (ADR-0008). Reuses `internal/filter/network/` (26.1/26.2 `TerminalFilter`; 27 ADR-0219 override seam — the routing precedent), `internal/cluster/` (`Manager.Get` host-resolution + `Cluster.Dial` TCP dial path + the per-cluster traffic-stat roster), `internal/filter/tcpproxy/` (the terminal-dial PRECEDENT the seam parallels), `internal/stats/` (06.1 counters + gauges + `IsValidName`), the differential harness + `fixture.StatsAsserter`. **ZERO new go.mod dependencies** (redis_proxy is a CORE `/envoy` extension — UNLIKE kafka_broker's `/contrib`; the RESP codec is in-house byte scanning, no Redis client library; AMEND-R1).

**Authored:** 2026-06-08. **Empirical-pin probe date (inherited):** 2026-06-08 (parent SPEC §11; NOT re-executed this session).

---

## 1. Purpose / Mission

Phase 32.1 delivers the redis_proxy seam + codec + round-trip foundation (parent §3.2 item "32.1"):

1. **The upstream connection-pool / cluster-routing SEAM (ADR-0230)** — the framework's SIXTH structural extension, in the existing `internal/filter/network/` package (NOT a new package; parent §4.1). The FIRST seam that lets a network filter ACTIVELY manage an upstream connection (dial + per-downstream lifecycle + FIFO/positional reply correlation) rather than observe a `tcp_proxy`-terminated one. It builds ON the as-built `TerminalFilter.Handle` (UNCHANGED) + reuses `cluster.Cluster.Dial`. Redis-scoped (no thrift generalization; YAGNI — thrift reuses/extends it at its phase). §4 pins the exact exported surface + the one-upstream-conn-per-downstream-conn multiplexing model (D-P32-3) + anchors the ADR-0230 §Context.

2. **The `redisproxy` package foundation (ADR-0229)** — `internal/filter/network/redisproxy/`: TypeURL via `proto.MessageName` (§3.2; the `extensions.` lesson), config parse of `stat_prefix` + `settings.op_timeout` + `prefix_routes.catch_all_route.cluster` (the PGV-required boot-reject arms + the runtime missing-upstream reject, AMEND-R2) with the rest of the proto parse-accepted/deferred (§3.3), the **in-house RESP codec** (`resp.go` — request decode + reply decode + encode; value types `+`/`-`/`:`/`$`/`*` + null sentinels + inline commands; the streaming-reader partial-frame model, D-P32-4), the `TerminalFilter.Handle` command→reply pump (§3.7), and a minimal `catch_all` single-command round-trip (PING [local] + SET/GET [proxied]) proving the seam end-to-end.

3. **The integration surface** — (a) registration as the **10th `builtins.RegisterBuiltins` built-in** + the `redis_proxy/v3` blank-import in `internal/bootstrap/bootstrap.go` (ZERO new go.mod dep; R8); (b) the new BackendKind (the synthesized **`TCPRedisResponder`**, value 32 — §8.3); (c) fixtures **`0055-redis-roundtrip`** (cross-side `StatsAsserter` over flat `/stats` + the downstream-RESP byte-equivalence prong; PING + proxied round-trip arms) + **`0056-redis-boot-reject`** (the `stat_prefix`-required arm; D-P32-5); (d) the **ADR-0230 §Context** (at THIS SPEC commit) + the §Decision/§Consequences body (at the 32.1 IMPL per ADR-0044); the BEHAVIOR_CONTRACT 32.1 bundle; the STATE/ROADMAP advance (sub-row 32.1 `planned → in-progress` at THIS SPEC commit; `→ done` at the 32.1 IMPL — parent row 32 STAYS `in-progress`, the ROLLUP is 32.2's).

After phase 32.1 the project has: a bootable single-command `redis_proxy` terminal with a LIVE cross-side round-trip (PING local-reply + a proxied SET/GET) + the upstream-pool seam (the analogue thrift reuses); the RESP codec foundation 32.2 extends with the full command set; the downstream cx/rq counters (+10 fixed names created eager). 32.2 then completes the command set + the full stat roster (the EAGER per-command table + the splitter + REDIS_CLUSTER_STATS counters + the 4 gauges' inc/dec + the `redis.` Prometheus tag-extractor arm + the differential command matrix + the 41st fuzzer `FuzzRESPDecode`) + the parent-row-32 ROLLUP.

### 1.1 Parent AMENDs load-bearing for 32.1 (per parent SPEC §1.1)

- **AMEND-R1** (TypeURL carries `extensions.`; redis_proxy is CORE `/envoy v1.32.4` → ZERO new go.mod dep) — informs §3.2 + §3.8 + the Task-1 `proto.MessageName` + clean-`go mod tidy` gate (R8).
- **AMEND-R2** (`stat_prefix` + `settings` + `settings.op_timeout` ALL PGV-REQUIRED; `catch_all_route.cluster` PGV min-1; + a RUNTIME `cannot configure a redis-proxy without any upstream` reject; **unknown cluster refs TOLERATED at validate**; deferred fields parse-accept) — informs §3.3 + §6 (all parse code lands at 32.1).
- **AMEND-R5** (PING/AUTH answered LOCALLY, zero upstream; count `downstream_rq_total` but no `command.*`/upstream) — informs §3.6 + §7.3 + the §8.1 PING arm (the 32.1 local-reply subset is PING + AUTH, D-P32-6).
- **AMEND-R6** (the `upstream_cx_*`/`upstream_rq_*` traffic stats live under the existing CLUSTER scope `cluster.<name>.*` — NOT a new redis roster; the seam reuses the per-cluster roster `manager.go:97-108`) — informs §4 + §7.5 + the §8.1 proxied-arm upstream assertion.
- **AMEND-R8** (the seam model: one shared client per HOST multiplexed; FIFO positional pop-front; envoy-go MAY use one-conn-per-downstream) — the load-bearing input to §4 / D-P32-3.
- **AMEND-R9** (NO close-direction counters → close-direction-framework-zero-touch; `reference_close_direction_framework_gap` does NOT bite) — informs §2 + §4.4 (the 29.3 `CloseDirection` machinery is NOT consumed).

(AMEND-R3/R4/R7 are 32.2-scoped — the EAGER per-command roster, the `redis.` LABEL-HOISTED Prometheus arm, the fixed-15-name roster — and bound this sub-phase in §2 Non-purposes; the 32.1 fixed-roster subset is §7.2.)

### 1.2 32.1-SPEC-additive contributions (what this document pins beyond the parent)

- **§4 ANCHORS the ADR-0230 §Context** (the upstream connection-pool / cluster-routing seam) — deferred from the parent SPEC (BRAINSTORM §7; parent §10). The seam's exact API + multiplexing model is pinned HERE (D-P32-3).
- **D-P32-3 RESOLVED (§4.2): one-upstream-conn-per-downstream-conn; a synchronous single-flight pump (degenerate depth-1 FIFO); the seam consumes a DIAL CLOSURE** (`internal/filter/network/` gains NO `internal/cluster` import — the upstreamcluster.go decoupling discipline; the in-filter routing decision resolves catch_all→cluster, §4.3). The shared per-host pool + cross-downstream multiplexing + a >1-depth pending queue + the two-goroutine pipelined model (+ the ADR-0223 per-conn mutex) are DEFERRED (consume-at-second-consumer; thrift / a future latency-or-fan-out sub-phase extends).
- **D-P32-4 RESOLVED (§3.5): the streaming-reader partial-frame model.** redis_proxy is a TERMINAL filter that OWNS the conn → the RESP decoder reads incrementally from a `bufio.Reader` over the conn and BLOCKS on partial frames (no private high-water buffer, no `chainConsumed` tracking). This is STRUCTURALLY SIMPLER than — and explicitly contrasted with — the mongo/kafka observer-chain private-buffer-copy model (those cannot block-read a `tcp_proxy`-owned conn).
- **D-P32-1 RESOLVED (§7.2): EAGER at config parse.** The 32.1 fixed-roster subset (10 names: 6 downstream counters + 4 gauges) is created eagerly via `NewCounterIfAbsent`/`NewGaugeIfAbsent` at parse (the kafka/mongo precedent). The boot-window visibility difference vs upstream's per-connection creation (AMEND-R7 context) is UNOBSERVABLE to the differential (every `0055` assertion is post-connection).
- **D-P32-5 RESOLVED (§6.2 / §8.2): `0056` carries the `stat_prefix` arm ONLY** (the kafka `0054`/mongo `0050` precedent); the `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` reject arms land as code + unit tests at 32.1.
- **D-P32-6 (32.1 subset) RESOLVED (§3.6): PING + AUTH** are the 32.1 local-reply set (AMEND-R5); ECHO/TIME/QUIT/HELLO are 32.2 follow-ons (the FULL extent stays the 32.2-SPEC D-P32-6).
- **The unknown-catch_all-cluster TOLERANCE pin (§3.3).** UNLIKE `tcp_proxy` (which REJECTS an unknown cluster at `NewFilter`, `tcpproxy/filter.go:63-66`), redis_proxy must NOT reject an unknown `catch_all_route.cluster` at config time (AMEND-R2 arm C — validate-tolerated). The filter stores the cluster NAME + the captured `cluster.Manager` and resolves LAZILY at `Handle` (a missing cluster → an upstream error, never a boot-reject). This is a load-bearing departure from the tcp_proxy resolution discipline.
- **The 32.1 StatsAsserter scrape-mechanics pin (§8.1.2): FLAT admin `/stats`, NOT `/stats/prometheus`.** The `redis.` Prometheus tag-extractor arm (`internal/stats/name.go`) lands at 32.2 (parent §3.1). The `0055` driver's `AssertStats` performs its scrape-and-diff IN-BAND (`fixture.go:75-77` — the driver holds both admin addrs) → it scrapes the flat admin `/stats` text (`redis.<sp>.<leaf>: <v>`), which the reference AND envoy-go expose identically by internal name — so NO `redis.` prom arm is needed at 32.1. The `/stats/prometheus` label-aware comparison + the `redis.` arm upgrade are 32.2's.
- **The downstream-RESPONSE byte-equivalence prong (§8.1.1) — the §9 FIRST.** redis_proxy GENERATES the downstream response (proxied verbatim from upstream for data commands; locally for PING/AUTH). The `0055` driver asserts the RESP reply bytes are byte-identical cross-side (`reference_wire_format_both_sides_see_same_bytes`, extended to the response) IN ADDITION to the stat parity.
- **Parent D-question resolutions owned by 32.1** (§12.1): **D-P32-1** (EAGER), **D-P32-3** (seam API + one-conn-per-downstream), **D-P32-4** (streaming-reader reassembly), **D-P32-5** (`0056` = `stat_prefix` arm only), **D-P32-6** (32.1 subset = PING + AUTH). D-P32-2/7/8/9 stay 32.2-owned.

---

## 2. Non-purposes

Phase 32.1 does NOT extend any subsystem beyond the minimum needed to land the seam + codec + round-trip under ADR-0229/ADR-0230.

- **2.1 The full command set OUT OF SCOPE.** Only PING (local) + AUTH (local) + SET/GET (proxied) are exercised at 32.1. The full single-route command set + ECHO/TIME/QUIT/HELLO local-replies are **32.2** (parent §3.2). The pump dispatches "is this a local-reply command?" → local; else → proxy via the seam (a command does not need a per-command stat to be proxied at 32.1).
- **2.2 The per-command + splitter + REDIS_CLUSTER_STATS roster OUT OF SCOPE.** The EAGER `command.<cmd>.{total,success,error}` table over the static ~180-command list (AMEND-R3), the 2 `splitter.*` counters, the 3 REDIS_CLUSTER_STATS, and the 4 downstream gauges' INC/DEC are all **32.2**. At 32.1 the 4 gauges are CREATED (eager) but not incremented (§7.2).
- **2.3 The `redis.` Prometheus TAG-EXTRACTOR arm OUT OF SCOPE** (`internal/stats/name.go`; AMEND-R4). It lands at 32.2 with the dynamic command-name flattening. The 32.1 differential compares flat `/stats` internal names (§8.1.2). `internal/stats/name.go` is UNTOUCHED at 32.1.
- **2.4 The deferred active features OUT OF SCOPE (parse-accepted, behavior-deferred; parent §2.1).** `prefix_routes.routes[]` multi-cluster routing (only `catch_all_route` consumed); hash-ring sharding + `enable_redirection`/`enable_hashtagging`/`dns_cache_config`; multi-key fragmentation (MGET/MSET/DEL split-collate); downstream AUTH enforcement (the configured-password path — 32.1 answers AUTH with the no-password-set error only, AMEND-R5); `faults`; request mirroring; replica `read_policy`; the full `ConnPoolSettings` surface beyond `op_timeout`. Each parse-accepts standalone (AMEND-R2); their stat counters exist-at-0 (32.2 creation).
- **2.5 The pipelined / shared-pool seam OUT OF SCOPE (deferred at the seam; §4.4).** One upstream conn per downstream conn (no cross-downstream multiplexing); a synchronous single-flight pump (no >1-depth pending queue, no two-goroutine read/write split). The shared per-host `ClientImpl` (the upstream model, AMEND-R8) + the ADR-0223 per-conn mutex for a deep pending queue are the consume-at-second-consumer boundary (thrift / a latency-sensitive sub-phase). Recorded as a per-side behavioral divergence (D-P32-9 — pinned at the 32.2 SPEC where the command matrix exercises concurrent downstream conns).
- **2.6 Command-latency histograms OUT OF SCOPE** (`command.<cmd>.latency`) — deferred per ADR-0060; the coverage-boundary record lands in the 32.2 bundle.
- **2.7 Close-direction machinery NOT consumed (AMEND-R9).** `ALL_REDIS_PROXY_STATS` has no close-direction-keyed counter; the 29.3 `CloseDirection` seam is untouched. `reference_close_direction_framework_gap` does NOT bite phase 32.
- **2.8 Runtime-key gating unmirrored** — envoy-go has no runtime layer; behaves at key defaults (the envoy-go-strict departure recorded in the 32.1 BEHAVIOR_CONTRACT bundle).
- **2.9 No real-Redis-server fixtures; RESP2 only; no histograms; no per-route surface; no new conformance harness** — all per parent §2. The hermetic `TCPRedisResponder` is the only backend; RESP3 (`HELLO 3`) is out of scope (HELLO is a 32.2 local-reply).
- **2.10 The 41st fuzzer `FuzzRESPDecode` is 32.2's** (parent §3.1) — it lands with the full command matrix + the IsValidName disposition. 32.1 proves the decoder via unit tests (§16) only.

---

## 3. The `redisproxy` package (ADR-0229; 32.1 foundation)

NEW Go package `internal/filter/network/redisproxy/` (package `redisproxy`, single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy`/`kafkabroker` precedent). Implements `network.TerminalFilter` (the `*tcpproxy.Filter`/`*hcm.Filter` shape — `Handle(ctx, downstream net.Conn)`; one boot-parsed instance shared across connections, per-connection state on `Handle`'s stack).

### 3.1 File split (lands at IMPL; the kafkabroker/mongoproxy precedent)

| File | Responsibility |
|---|---|
| `doc.go` | package doc — the terminal redis_proxy; ADR-0229/ADR-0230 cross-refs; the 32.2 forward-pointers |
| `redisproxy.go` | `TypeURL` (via `proto.MessageName` — §3.2) + `NewFactory(cm, reg)` + the boot-parsed `filter` struct glue |
| `config.go` | `compiledConfig` + `parseConfig` (the PGV-required arms + the runtime no-upstream check + the deferred-field parse-accept + the `IsValidName(stat_prefix)` guard) + the PARSE-REJECT constants — §3.3 / §6 |
| `resp.go` | the in-house RESP codec (request decode + reply decode + encode; value types + null sentinels + inline; the streaming-reader reassembly) — §3.5 |
| `commands.go` | the 32.1 local-reply set (PING / AUTH) + the local-vs-proxy dispatch decision — §3.6 |
| `stats.go` | the 32.1 fixed-roster subset (6 counters + 4 gauges) eager creation + the inc accessors — §3.4 / §7.2 |
| `filter.go` | the `TerminalFilter.Handle` command→reply pump + the upstream-pool seam consumption + the downstream cx/rq lifecycle — §3.7 |
| `*_test.go` | per-file unit tests (§16) |

(The exact split is the 32.1 PLAN/IMPL's; the parent §4.2 anticipated `redisproxy.go`/`config.go`/`resp.go`/`stats.go`/`commands.go`/`filter.go` — `stats.go` here carries only the 32.1 subset; the EAGER per-command table in `commands.go`+`stats.go` is 32.2.)

### 3.2 TypeURL + factory shape

```go
// TypeURL is derived via proto.MessageName, NEVER a hand-typed docs string
// (reference_network_filter_typeurl_extensions; the kafkabroker.go/mongoproxy.go
// precedent). redis_proxy/v3 is CORE /envoy v1.32.4 (AMEND-R1).
var TypeURL = "type.googleapis.com/" + string(proto.MessageName(&redis_proxyv3.RedisProxy{}))

// NewFactory returns the redisproxy NetworkFilterFactory. UNLIKE the
// stats-only zookeeper/mongo/kafka factories, redisproxy needs BOTH the cluster
// Manager (to resolve catch_all → *cluster.Cluster at Handle time — the tcp_proxy
// precedent) AND the stats registry (the redis.<sp> roster). Both are closure-
// captured from builtins.Deps (the network FactoryCtx carries neither).
func NewFactory(cm *cluster.Manager, reg *stats.Registry) network.NetworkFilterFactory
```

Pinned: `proto.MessageName` resolves to `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy` (parent §5.1; the `extensions.` segment). The IMPL Task-1 pinning test asserts `TypeURL` ends in the parent §5.1 string (derivation by `proto.MessageName`, assertion against the empirically-pinned literal) AND that a clean `go mod tidy` adds ZERO modules (R8). The filter-chain registration `name` is `envoy.filters.network.redis_proxy`. Go package alias `redis_proxyv3`.

The factory parses + validates ONCE at boot (ADR-0079 two-step factory), resolves the `catch_all_route.cluster` NAME (NOT the `*cluster.Cluster` — lazy, §3.3), and creates the 10 fixed stats ONCE per distinct `stat_prefix` at parse time (§7.2; D-P32-1 eager). The returned `FilterInstanceFactory` yields the SHARED boot-parsed `*filter` per accepted connection (redis_proxy is conn-stateless at the struct level — per-connection state is `Handle`'s `bufio.Reader` + the `*network.UpstreamConn`, both stack/`Handle`-local).

### 3.3 Config parse (the PGV-required arms + the runtime check + parse-accept-deferred)

Parses the full proto (parent §5 roster, inherited verbatim). The 32.1 disposition:

| Field | 32.1 parse behavior |
|---|---|
| `stat_prefix` | REQUIRED (PGV min-1-rune mirror) → boot-reject (§6.2; the `0056` fixture arm) + `stats.IsValidName(stat_prefix)` guard (a metric-name-invalid prefix → reject at the user-input boundary, the cluster `manager.go:205` / `reference_dynamic_stat_name_charset_guard` precedent) |
| `settings` | REQUIRED (PGV `value is required` — the whole `ConnPoolSettings` message is mandatory, AMEND-R2) → boot-reject |
| `settings.op_timeout` | REQUIRED (PGV `value is required`, AMEND-R2) → boot-reject. PARSED + stored; the timeout's CONSUMPTION (bounding the upstream round-trip) is a 32.2 concern (the MVP round-trip is synchronous against a hermetic responder — no timeout path exercised at 32.1; recorded as a parse-but-not-yet-consumed note) |
| `prefix_routes.catch_all_route.cluster` | PGV min-1 when a catch_all is present (§5.4) → boot-reject; the cluster NAME stored. **Unknown cluster ref is NOT rejected** (AMEND-R2 arm C — validate-tolerated; resolved lazily at `Handle`, §3.7) |
| `prefix_routes` (omitted / `{}` / no upstream) | RUNTIME missing-upstream reject `cannot configure a redis-proxy without any upstream` (AMEND-R2 — NEITHER `catch_all_route` NOR `routes[]` supplies an upstream); NO `Proto constraint validation failed` prefix |
| `prefix_routes.routes[]`, `settings.*` (beyond op_timeout), `latency_in_micros`, `downstream_auth_*`, `faults`, `external_auth_provider` | parse-accept (stored or ignored), behavior-deferred (parent §2.1 / §5) |

**The unknown-catch_all-cluster tolerance (§1.2 pin).** The factory stores `catchAllCluster string` + the captured `cm *cluster.Manager`; it does NOT call `cm.Get` at boot (that would reject unknown clusters, breaking AMEND-R2 arm C). Resolution is lazy at `Handle` (the first proxied command): `cm.Get(catchAllCluster)` → on miss, an upstream error (close / a future `-ERR`); the `0055` fixture's cluster always exists. A unit test proves an unknown catch_all cluster boots clean (no reject) and that a proxied command against it fails gracefully (no panic, connection closes).

### 3.4 The 32.1 fixed-roster subset (`stats.go`)

The 32.1 subset of the parent §7.2 fixed-15 roster: the **6 downstream counters + the 4 downstream gauges** = **10 names** under `redis.<stat_prefix>.` (the 2 `splitter.*` + 3 REDIS_CLUSTER_STATS = 5 names + the EAGER per-command table are 32.2). A `rosterSuffixes()` table (the `kafkabroker/stats.go` / `zookeeperproxy/stats.go:159-175` shape) producing the exact suffixes:

- **Counters (6):** `downstream_cx_total`, `downstream_cx_drain_close`, `downstream_cx_protocol_error`, `downstream_cx_rx_bytes_total`, `downstream_cx_tx_bytes_total`, `downstream_rq_total`.
- **Gauges (4, Accumulate — the project's 2nd mirrored gauge family after mongo's `op_query_active`):** `downstream_cx_active`, `downstream_cx_rx_bytes_buffered`, `downstream_cx_tx_bytes_buffered`, `downstream_rq_active`.

Created EAGERLY at config parse (D-P32-1; `reg.NewCounterIfAbsent("redis.<sp>." + suffix)` + `reg.NewGaugeIfAbsent(...)` — idempotent across listeners sharing a `stat_prefix`, the kafka/mongo precedent). A byte-stable `TestStatRoster32_1_MatchesUpstream` test pins the 10 names against a golden subset transcribed from `ALL_REDIS_PROXY_STATS` (the R2 subset; the full-15 + per-command roster test is 32.2's). The inc accessors land for the 32.1-incremented subset (§7.2).

### 3.5 The in-house RESP codec (`resp.go`; D-P32-4 RESOLVED)

Mirrors upstream `codec_impl.cc` framing exactly (parent §11.5 — `reference_wire_format_both_sides_see_same_bytes`). All reads are reader-based.

**Value-type framing (parent §11.5):** simple-string `+<text>\r\n`; error `-<text>\r\n`; integer `:<n>\r\n`; bulk-string `$<len>\r\n<len bytes>\r\n`; null bulk `$-1\r\n`; array `*<n>\r\n<n elements>`; null array `*-1\r\n`; **inline commands** (no leading type byte — a space-separated token line terminated by `\r\n` or bare `\n`, parsed as a command array, e.g. `PING\r\n`).

**Partial-frame reassembly (D-P32-4 RESOLVED — the streaming-reader model).** redis_proxy is a TERMINAL filter that OWNS the `net.Conn`. The codec reads from a `bufio.Reader` over the conn (downstream request side) or over the seam's upstream conn (reply side) and reads INCREMENTALLY: `ReadByte()` for the type marker, `ReadString('\n')` for `\r\n`-terminated lines, `io.ReadFull` for bulk payloads. A partial frame simply BLOCKS on the next `Read` (the terminal can block — it owns the conn and runs a blocking serve loop, the `tcp_proxy.Handle` precedent). `io.EOF` at a frame boundary → clean connection end (`Handle` returns); `io.ErrUnexpectedEOF` mid-frame → a protocol/transport error (connection close; the `downstream_cx_protocol_error` increment is 32.2). **This is STRUCTURALLY SIMPLER than — and explicitly contrasted with — the mongo/kafka observer-chain private-buffer-copy + `chainConsumed` high-water model** (`reference_network_chain_terminal_handoff_ends_ondata`): those filters OBSERVE a `tcp_proxy`-owned chain, get fed COPIES of chain-buffer bytes via `OnData`, and CANNOT block-read the conn → they must privately buffer partial frames. redis_proxy owns the conn → no private buffer, no high-water tracking, no `OnData`. The decoder API shape (anticipated; IMPL finalizes — D-S32.1-2):

```go
// decodeRequest reads ONE request frame (inline OR array-of-bulk-strings) from
// r and returns the UPPERCASED command name (for dispatch) + the RAW frame
// bytes (forwarded VERBATIM upstream when proxied). Blocks on partial frames.
func decodeRequest(r *bufio.Reader) (cmd string, raw []byte, err error)

// decodeReply reads ONE complete reply frame (any value type incl. null
// sentinels + nested arrays) from r and returns its RAW bytes (forwarded
// VERBATIM downstream — the byte-equivalence prong, §8.1.1). It parses only
// enough to find the frame boundary; it does not re-encode.
func decodeReply(r *bufio.Reader) (raw []byte, err error)

// encode* build the LOCAL-reply bytes (PING +PONG; the AUTH no-password error).
// Byte-stable constants (the +PONG / -ERR wording from parent §11.5/§11.6).
```

Malformed frames (bad type byte, negative non-`-1` length, length overflow, truncated line) → a decode error the pump maps to connection close (the `downstream_cx_protocol_error` + `splitter.invalid_request` increments are 32.2). The decoder NEVER panics on arbitrary bytes (proven by unit tests at 32.1; the `FuzzRESPDecode` fuzzer is 32.2).

### 3.6 The 32.1 local-reply set + dispatch (`commands.go`; D-P32-6 32.1 subset)

The pump's per-request dispatch decision (AMEND-R5):

- **PING** → answered LOCALLY with `+PONG\r\n`; the argument (if any) is IGNORED (`PING foo` still returns `+PONG\r\n` — does NOT echo, the reference behavior, parent §11.6); ZERO upstream traffic.
- **AUTH** (no `downstream_auth_password` configured — the 32.1 posture, §2.4) → answered LOCALLY with `-ERR Client sent AUTH, but no password is set\r\n` (byte-stable, parent §11.6); ZERO upstream traffic.
- **Any other command** (SET/GET/… at 32.1) → PROXIED via the seam (§4 / §3.7). A command does NOT need a per-command stat entry to be proxied at 32.1 (the per-command roster is 32.2); the pump forwards the raw request bytes verbatim upstream and forwards the raw reply verbatim downstream.

ECHO/TIME/QUIT/HELLO are 32.2 local-reply follow-ons (the FULL extent stays the 32.2-SPEC D-P32-6). Command-name matching is ASCII-case-insensitive (RESP commands are case-insensitive; the dispatch uppercases the decoded name — §3.5).

### 3.7 The `TerminalFilter.Handle` command→reply pump (`filter.go`)

```go
type filter struct {
	network.Marker            // sealed-marker embed → satisfies network.TerminalFilter
	cfg *compiledConfig       // shared, boot-parsed (stat_prefix, catchAllCluster, op_timeout, the roster stats)
	cm  *cluster.Manager      // for lazy catch_all resolution at Handle (the tcp_proxy precedent; §3.3)
}

var _ network.TerminalFilter = (*filter)(nil)
```

**`Handle(ctx, downstream net.Conn)`** (the `tcp_proxy.Handle` shape, `tcpproxy/filter.go:101`):

1. `defer downstream.Close()`; `cfg.downstreamCxTotal.Inc()` (+ the gauge inc/dec is 32.2 — §7.2). Build `dr := bufio.NewReader(downstream)`.
2. Lazily prepare the upstream seam: on the FIRST proxied command, resolve `cm.Get(cfg.catchAllCluster)` → `*cluster.Cluster` (a miss → close the connection, no panic — §3.3); build the dial closure `func(ctx) (net.Conn, error) { c, _, err := cl.Dial(ctx); return c, err }` (the `Endpoint` discarded — §4.2) + the per-request hook `cl.IncUpstreamRqTotal` (AMEND-R6); `up := network.NewUpstreamConn(dial, cl.IncUpstreamRqTotal)`; `defer up.Close()`.
3. Serve loop: `cmd, raw, err := decodeRequest(dr)`; on `io.EOF` → return (clean close); on decode error → return (close; the protocol_error counter is 32.2). `cfg.downstreamRqTotal.Inc()`; account `downstream_cx_rx_bytes_total += len(raw)` (§7.2).
   - **PING/AUTH** (§3.6) → write the local reply to `downstream`; account `downstream_cx_tx_bytes_total += len(reply)`; loop. ZERO upstream.
   - **else (proxied)** → ensure the seam is dialed (step 2 lazily on first proxied); `up.Send(ctx, raw)` (writes raw upstream, Incs `cluster.upstream_rq_total` via the hook; `cluster.upstream_cx_total`/`active` Inc'd by `Cluster.Dial` on the first dial); `reply, err := decodeReply(up.Reader())` → write `reply` verbatim to `downstream`; account `downstream_cx_tx_bytes_total += len(reply)`; loop. The synchronous single-flight ordering (one `Send`+one `decodeReply` per request before the next request) is the FIFO/positional correlation invariant (R4; §4.2).
4. EOF / error → `Handle` returns; the deferred `downstream.Close()` + `up.Close()` run.

NO halt path (the MVP injects no fault delay — the ADR-0226 async halt/resume seam is NOT consumed). NO chain Buffer, NO `OnData`/`OnWrite`/`OnDestroy` (those are the ReadFilter/WriteFilter observer surface — redis_proxy is a TERMINAL). Concurrency: each `Handle` runs on its own goroutine with its own `*UpstreamConn`; the shared `*filter` is read-only after boot; the roster `*stats.Counter`/`*stats.Gauge` are atomic. NO per-connection mutex at 32.1 (the single-flight pump is single-goroutine; the ADR-0223 mutex arrives only WITH the deferred two-goroutine pipelined model — §4.4).

### 3.8 The 10th built-in + bootstrap blank-import

- `internal/filter/network/builtins/builtins.go`: `reg.Register(redisproxy.TypeURL, redisproxy.NewFactory(deps.ClusterManager, deps.StatsRegistry))` — the 10th registration, after kafkabroker (`builtins.go:82`). UNLIKE the stats-only kafka/mongo/zookeeper registrations, redisproxy passes BOTH `deps.ClusterManager` (lazy catch_all resolution) + `deps.StatsRegistry` (the roster) — the `tcpproxy.NewNetworkFactory(deps.ClusterManager, …)` + the stats-capture precedents combined. The package doc-comment "nine built-in network filters (…)" (line 1) + the `RegisterBuiltins` doc (line 43) update to "ten … (…, redis_proxy)". Registration order is behavior-neutral (ADR-0072).
- `internal/bootstrap/bootstrap.go`: blank-import `_ ".../envoy/extensions/filters/network/redis_proxy/v3"` (after the kafka_broker import — required for `@type` Any resolution at config load; ZERO new go.mod dep, AMEND-R1; differential bootstraps need ≥1 cluster per `reference_network_filter_typeurl_extensions`).

---

## 4. The upstream connection-pool / cluster-routing seam (ADR-0230; §Context ANCHORED here)

The framework's SIXTH structural extension, in the existing `internal/filter/network/` package (parent §4.1 — NOT a new package). Anticipated file: `upstreampool.go`. This section pins the exact exported surface + the multiplexing model (D-P32-3) and is the blueprint for the ADR-0230 §Context (§10).

### 4.1 As-built anchors the seam builds on (verified this SPEC session at master tip `617d8c3`)

- `internal/filter/network/terminal.go:30-49` — `TerminalFilter.Handle(ctx, downstream net.Conn)` (UNCHANGED; redis_proxy IS a terminal filter).
- `internal/cluster/manager.go:110-114` — `Manager.Get(name) (*Cluster, bool)` host-resolution lookup (the same `tcp_proxy.NewFilter` uses at `filter.go:63`; redis_proxy uses it LAZILY at Handle, §3.3).
- `internal/cluster/cluster.go:198-223` — `Cluster.Dial(ctx) (net.Conn, Endpoint, error)`: picks a host via the round-robin LB, dials TCP (+ optional TLS handshake), Incs `cluster.<name>.upstream_cx_total` + `upstream_cx_active`, and returns a `*connWithGauge` whose `Close()` Decs the active gauge once. The seam reuses this verbatim (the `Endpoint` return is discarded — §4.2).
- `internal/cluster/cluster.go:134` — `Cluster.IncUpstreamRqTotal()` (Incs `cluster.<name>.upstream_rq_total`; the per-proxied-request hook, AMEND-R6).
- `internal/filter/tcpproxy/filter.go:101-159` — the terminal dial + bidirectional `io.Copy` pump PRECEDENT (redis_proxy replaces the raw pump with a RESP request→reply round-trip).
- `internal/filter/network/upstreamcluster.go` — the ADR-0219 override seam (context-channel, NO `cluster` import); the decoupling discipline §4.2 preserves.

### 4.2 The seam surface + multiplexing model (D-P32-3 RESOLVED)

**Multiplexing model: ONE upstream connection per downstream connection** (AMEND-R8 — "the simplest faithful model … trivially-correct positional correlation, no cross-downstream multiplexing"). The upstream conn is LAZILY dialed on the first PROXIED (data) command — a PING/AUTH-only downstream connection NEVER dials (AMEND-R5; the `0055` PING arm asserts `cluster.<name>.upstream_cx_total == 0`). It is closed when `Handle` returns (downstream close).

**Pump model: synchronous single-flight** — the `Handle` goroutine reads one request, `Send`s it upstream, reads exactly one reply, writes it downstream, then loops. This is a DEGENERATE depth-1 FIFO pending queue — the load-bearing FIFO/positional pop-front CONTRACT (R4; parent §4.1 item 4) satisfied trivially. A pipelining client (multiple requests in one write before reading) is handled correctly: the requests sit buffered in `dr`/the TCP receive buffer and are processed in order, replies returned in order. The serialization (no concurrent in-flight upstream requests) is a latency-only divergence from the reference's pipelined client (NOT observable in reply bytes or counters; §4.4 / D-P32-9).

**Decoupling (the upstreamcluster.go discipline): the seam consumes a DIAL CLOSURE, NOT `*cluster.Cluster`.** `internal/filter/network/` gains NO `internal/cluster` import (which would couple the universally-imported network package to cluster). The `redisproxy` filter — which imports BOTH network + cluster, like `tcpproxy` — resolves catch_all → `*cluster.Cluster` (the in-filter routing decision, §4.3) and supplies the seam a `func(ctx) (net.Conn, error)` over `Cluster.Dial` (Endpoint discarded) + a per-request `func()` hook over `Cluster.IncUpstreamRqTotal`. (Verified acyclic regardless — `internal/cluster` does not import `internal/filter/network` — but the closure keeps the dependency OUT of the core package; the IMPL `go build` confirms.)

Anticipated exported surface (the EXACT signatures are the IMPL pin — D-S32.1-1; the SHAPE + contract are pinned here):

```go
// internal/filter/network/upstreampool.go (ADR-0230)
package network

// UpstreamDialFunc resolves + dials one upstream connection. redisproxy supplies
// a closure over the boot-resolved cluster's Cluster.Dial (Endpoint discarded) —
// keeping internal/filter/network free of an internal/cluster import.
type UpstreamDialFunc func(ctx context.Context) (net.Conn, error)

// UpstreamConn is one downstream connection's dedicated upstream connection,
// with FIFO/positional reply correlation (RESP carries no correlation id). The
// MVP is one-conn-per-downstream + synchronous single-flight (AMEND-R8); the
// shared per-host pool + a >1-depth pending queue + the two-goroutine pipelined
// model are DEFERRED (§4.4; consume-at-second-consumer — thrift extends).
type UpstreamConn struct { /* dial, onRequest hook, lazily-dialed conn + bufio.Reader, single-flight guard */ }

// NewUpstreamConn binds a dial closure + a per-proxied-request hook (the filter
// passes cluster.IncUpstreamRqTotal — the AMEND-R6 upstream_rq_total Inc).
func NewUpstreamConn(dial UpstreamDialFunc, onRequest func()) *UpstreamConn

// Send forwards one request's raw bytes upstream (lazy-dialing on the first call
// — that first Dial Incs cluster.upstream_cx_total/active; every call fires the
// onRequest hook → cluster.upstream_rq_total) and enqueues a pending marker.
func (u *UpstreamConn) Send(ctx context.Context, reqBytes []byte) error

// Reader returns the buffered reader the codec decodes the reply frame from
// (stable across Sends; valid after the first Send). Positional correlation:
// the synchronous pump reads exactly one reply per Send (depth-1 FIFO; R4).
func (u *UpstreamConn) Reader() *bufio.Reader

// Close closes the upstream conn (idempotent; the Cluster.Dial connWithGauge
// Decs upstream_cx_active once). Called from the filter's Handle defer.
func (u *UpstreamConn) Close() error
```

Redis-scoped (Q-pool-seam): catch_all single-cluster routing (the FILTER's decision, §4.3), one logical request stream per downstream connection, lazy single dial — NO thrift-specific generalization (no method-routing abstraction, no pluggable-codec interface, no shared pool). The seam knows NOTHING about RESP framing (the codec reads reply frames from `Reader()`); it is a connection-lifecycle + ordered-round-trip primitive a future protocol can reuse.

### 4.3 Routing stays in-filter

The catch_all → cluster routing DECISION lives in `redisproxy` (parent §4.3 "the in-filter routing decision"), NOT the seam: `config.go` stores `catch_all_route.cluster`; `Handle` resolves it via `cm.Get` (lazy; tolerant of an unknown cluster, §3.3). `prefix_routes.routes[]` longest-prefix routing (the upstream `router_impl.cc` model, parent §11.3) is 32.2+/deferred — it would extend the in-filter routing table, not the seam's API. The ADR-0219 override seam (`upstreamcluster.go`) is the nearest precedent (a per-connection routing decision feeding the terminal) and is UNTOUCHED.

### 4.4 What the seam DEFERS (the consume-at-second-consumer boundary)

Pinned so the seam's redis-scoped narrowness is an explicit record, not an omission:

- **The shared per-host pool** (the upstream `client_map_[host] → one ClientImpl` multiplexed across all downstream conns, AMEND-R8/parent §11.3) — DEFERRED. The MVP dials one conn per downstream conn. Consequence: `cluster.<name>.upstream_cx_total` diverges per-side when MULTIPLE downstream connections are live concurrently (the reference shares one upstream client; envoy-go dials one each) — pinned per-side at the 32.2 SPEC (D-P32-9). For a SINGLE-downstream-connection arm (the `0055` 32.1 round-trip), both sides dial exactly once → `upstream_cx_total == 1` cross-side equality HOLDS.
- **The >1-depth FIFO pending queue + the two-goroutine read/write pump + the ADR-0223 per-connection mutex** — DEFERRED. The MVP's synchronous single-flight pump needs none (single goroutine, depth-1). The deeper queue is needed only for concurrent in-flight upstream pipelining (latency) or multi-key fan-out/collate (deferred, parent §2.1).
- **`op_timeout` enforcement** (bounding the round-trip + the timeout reply) — PARSED + stored at 32.1, CONSUMED later (no timeout path against the hermetic responder). Recorded in the BEHAVIOR_CONTRACT 32.1 bundle.
- **`max_upstream_unknown_connections` / `connection_rate_limit`** + their REDIS_CLUSTER_STATS counters — deferred (their counters are 32.2 creation, exist-at-0).

These map 1:1 onto the deferred active features (parent §2.1); each WOULD extend this seam, per the consume-at-consumer discipline.

---

## 5. Proto-field roster (cross-reference parent §5)

INHERITED VERBATIM from parent §5.1 (TypeURL) + §5.2 (`RedisProxy` top-level) + §5.3 (`RedisProxy_ConnPoolSettings`) + §5.4 (`PrefixRoutes` + `Route`). No re-transcription here. The 32.1 IMPL Task-1 gate re-confirms `proto.MessageName` + the field roster against go-control-plane `/envoy` v1.32.4 in-tree (`~/go/pkg/mod/.../envoy@v1.32.4/extensions/filters/network/redis_proxy/v3/`) + a clean `go mod tidy` (ZERO new module, R8) before writing the parser, per the 26.x–31 Task-1 precedent.

---

## 6. PARSE-REJECT roster (cross-reference parent §6; ALL parse code lands at 32.1)

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a `TestParseRejectConstants_ByteStable` table test at IMPL. The error prefix for all redisproxy arms is **`redis_proxy: `** (mirrors `kafka_broker: `/`mongo_proxy: `/`zookeeper_proxy: `; exact wording finalized at IMPL — D-S32.1-3). Phase 32 has NO departure-class rejects (parent §6.1) — every arm mirrors an upstream PGV/config-load failure; the boot-reject differential matches a per-side boot-stderr SUBSTRING (not exact cross-impl string equality — the kafka `0054`/mongo `0050` precedent; the C++ `value length must be at least 1 characters` vs Go `1 runes` idiom difference, parent §6.1).

### 6.1 The load-bearing 32.1 arm (fixture-proven)

- **`redis-proxy-stat-prefix-required`** — missing/empty `stat_prefix` → boot-reject (PGV min-1-rune mirror, §5.2). The `0056` fixture arm (§8.2): BOTH sides reject at boot; common stderr substring `stat_prefix`.

### 6.2 The remaining 32.1 arms (code + unit tests; NOT fixture arms — D-P32-5)

All parse code + unit tests land at 32.1 (the parse path is whole):

- **`redis-proxy-settings-required`** — `settings` absent → boot-reject (PGV `value is required`, AMEND-R2). Reference: `… RedisProxyValidationError.Settings: value is required`.
- **`redis-proxy-op-timeout-required`** — `settings` present, `settings.op_timeout` absent → boot-reject (PGV `value is required`, AMEND-R2). Reference: `… ConnPoolSettingsValidationError.OpTimeout: value is required`.
- **`redis-proxy-no-upstream`** — RUNTIME (non-PGV) reject when NEITHER `catch_all_route` NOR `routes[]` supplies an upstream (fires for `prefix_routes` omitted AND `prefix_routes: {}` empty). Reference: `cannot configure a redis-proxy without any upstream` (NO `Proto constraint validation failed` prefix).
- **`redis-proxy-catch-all-cluster-required`** — a `catch_all_route` present with an empty `cluster` → boot-reject (PGV min-1, §5.4).

Per D-P32-5 (RESOLVED): `0056` carries the `stat_prefix` arm ONLY; these four stay unit-test-only at 32.1 (the kafka `0054`/mongo `0050` precedent — one load-bearing required-field fixture arm; the rest unit-armed).

### 6.3 NOT rejects (parse-accept; AMEND-R2)

- Unknown cluster ref in `catch_all_route.cluster` (and `external_auth_provider.grpc_service`) → NOT a reject at config time (validate-tolerated — §3.3; the subject MUST boot clean on an unknown catch_all cluster, a unit test).
- All deferred fields (`faults`, `downstream_auth_*`, `external_auth_provider`, `enable_redirection`-without-`dns_cache_config`, `routes[]`, `read_policy`, …) parse-accept standalone (parent §2.1 / §5).
- Framework-level: unknown network-filter `typed_config` type_url → the existing unified boot-reject (redisproxy joins the registry; no new arm).

---

## 7. Stat surface (cross-reference parent §7; the 32.1 subset)

### 7.1 Scope shape — `redis.<stat_prefix>.<leaf>` (AMEND-R7, inherited)

envoy-go mirrors upstream's internal naming exactly (the flat-`/stats` `StatsAsserter` depends on it — §8.1.2). Internal registration name = `redis.<stat_prefix>.<suffix>` for the 10 fixed 32.1 names. The `redis.` Prometheus tag-extractor arm (`internal/stats/name.go`) is 32.2 (§2.3) — the 32.1 differential compares flat admin `/stats` internal names, which need no arm.

### 7.2 The 32.1 roster subset + creation/increment posture (D-P32-1 RESOLVED: EAGER)

The 10 names of §3.4, all CREATED EAGERLY at config parse (D-P32-1). The 32.1 increment split:

| Name | Kind | Created | Incremented at 32.1 |
|---|---|---|---|
| `downstream_cx_total` | counter | 32.1 (eager) | **yes** (+1 per accepted downstream connection) |
| `downstream_cx_rx_bytes_total` | counter | 32.1 (eager) | **yes** (+= raw request bytes read) |
| `downstream_cx_tx_bytes_total` | counter | 32.1 (eager) | **yes** (+= reply bytes written) |
| `downstream_rq_total` | counter | 32.1 (eager) | **yes** (+1 per decoded request — incl. PING/AUTH, AMEND-R5) |
| `downstream_cx_drain_close` | counter | 32.1 (eager) | no (drain path — 32.2) |
| `downstream_cx_protocol_error` | counter | 32.1 (eager) | no (decode-error path — 32.2) |
| `downstream_cx_active` | gauge | 32.1 (eager) | no (inc/dec — 32.2; the 2nd mirrored gauge family) |
| `downstream_cx_rx_bytes_buffered` | gauge | 32.1 (eager) | no (32.2) |
| `downstream_cx_tx_bytes_buffered` | gauge | 32.1 (eager) | no (32.2) |
| `downstream_rq_active` | gauge | 32.1 (eager) | no (32.2) |

The 4 "cx/rq" counters increment at 32.1 (the round-trip + raw-socket byte accounting, parent §3.1 "incremented 32.1 (cx/rq for the round-trip)"). `drain_close`/`protocol_error` + the 4 gauges' inc/dec are 32.2 (parent §3.1). The byte counters are raw downstream-socket byte counts (every read/write len), so they match cross-side EXACTLY (both sides see identical wire bytes — the byte-equivalence prong, §8.1.1).

### 7.3 PING/AUTH local-reply stat accounting (AMEND-R5)

A PING/AUTH connection increments `downstream_cx_total` + `downstream_rq_total` (+ the rx/tx byte counters) but emits NO `command.*` (the per-command roster is 32.2; PING/AUTH are never in it) and NO upstream cx/rq (answered locally — the seam never dials). The `0055` PING arm asserts: `downstream_cx_total`/`downstream_rq_total` +1, `cluster.<name>.upstream_cx_total`/`upstream_rq_total` == 0, the `+PONG\r\n` reply bytes (the byte-equivalence prong). The proxied SET/GET arm asserts `downstream_cx`/`rq` increments, `cluster.<name>.upstream_cx_total == 1` (one lazy dial) + `upstream_rq_total == 2` (SET + GET), and the proxied reply bytes (`+OK\r\n`, `$3\r\nbar\r\n`).

### 7.4 Upstream traffic stats (AMEND-R6) — the existing CLUSTER roster, reused

`upstream_cx_total`/`upstream_cx_active`/`upstream_rq_total` are the cluster's OWN traffic stats under `cluster.<name>.*` (`internal/cluster/manager.go:97-108`), NOT a redis roster. The seam reuses them: `Cluster.Dial` Incs `upstream_cx_total`/`active` on the lazy dial; the per-request hook (`Cluster.IncUpstreamRqTotal`) Incs `upstream_rq_total` per proxied request. The 3 REDIS_CLUSTER_STATS (`upstream_cx_drained`/`max_upstream_unknown_connections_reached`/`connection_rate_limited`) are the only redis-specific upstream/pool counters — they are 32.2 creation (exist-at-0 for the MVP). The HTTP-shaped `cluster.<name>.upstream_rq_2xx..5xx` have no RESP analog (they stay at 0; pinned per-side — they are NOT asserted in the `0055` redis arms).

### 7.5 Project stat-count delta — 536 → **546** at 32.1

The 10 fixed names land at 32.1 (creation parity; +10). The remaining 5 fixed names (2 `splitter.*` + 3 REDIS_CLUSTER_STATS) + the EAGER per-command roster (~180 × 3 ≈ 540) land at 32.2 (parent §7.5 — the project's largest single-phase jump). The BEHAVIOR_CONTRACT stat table gains the 10 rows in the 32.1 bundle (§9). Departures (32.1 bundle): the boot-window eager-vs-per-connection creation difference (unobservable to the differential); `op_timeout` parsed-not-consumed; runtime-keys-at-defaults; the gauges created-not-incremented (the inc/dec is 32.2); the close-direction-zero-touch posture (AMEND-R9).

---

## 8. Differential fixture taxonomy (+2; cross-reference parent §8)

Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE dirs. Per `reference_differential_asserter_dispatch`: subject-side stat assertions use `fixture.StatsAsserter` (`fixture.go:75-77`; cross-side-path-only dispatch) and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). Numbering continues from `0054` (the master-tip tail `0054-kafka-boot-reject`): 32.1 lands **`0055` + `0056`** → 56 → **58** dirs.

**The §9 FIRST — downstream-RESPONSE byte-equivalence as a load-bearing prong.** Unlike the sniffer rows (bytes pass through unchanged; the stat comparison IS the proof), redis_proxy GENERATES the downstream response → the proof is TWO-pronged: (1) the RESP bytes returned to the client are byte-identical cross-side (`reference_wire_format_both_sides_see_same_bytes`, EXTENDED to the response the proxy generates); (2) cross-side `StatsAsserter` over the `redis.<sp>.` + `cluster.<name>.*` rosters.

**Fixture-design caveats (parent §8 + §11.2):** (i) the backend MUST be a real listening TCP RESP responder reachable via a STRICT_DNS hostname on a shared bridge network in a LIVE-Envoy probe — but the differential harness boots the reference image directly, so the `TCPRedisResponder` (§8.3) is the in-process backend the runner already wires (the `TCPKafkaResponder`/`TCPMongoResponder` precedent); verify the round-trip ran via `cluster.<name>.upstream_rq_total > 0` + a non-empty downstream RESP reply; (ii) drive RESP2; (iii) PING/AUTH arms expect ZERO upstream traffic (AMEND-R5).

### 8.1 `0055-redis-roundtrip` (32.1 + 32.2; cross-side; multi-arm)

**Topology.** Chain `[redis_proxy]` as the TERMINAL on BOTH sides (the contrib reference Envoy + envoy-go subprocess; NO `tcp_proxy` — redis_proxy terminates), `catch_all_route.cluster` → ONE cluster → the new `TCPRedisResponder` backend (§8.3); the driver acts as a RESP client. The 32.1 arms (32.2 extends with the command matrix + splitter arms — parent §8.1):

1. **PING (local-reply; 32.1)** — `PING\r\n` (inline) + `*1\r\n$4\r\nPING\r\n` (array) → `+PONG\r\n` reply byte-equivalence; `redis.<sp>.downstream_rq_total` +1 per request; `cluster.<name>.upstream_cx_total`/`upstream_rq_total` == 0 (AMEND-R5; the seam never dials on a PING-only connection).
2. **proxied round-trip (32.1)** — `*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n` → `+OK\r\n`; then `*2\r\n$3\r\nGET\r\n$3\r\nfoo\r\n` → `$3\r\nbar\r\n` (same connection) → downstream-RESP byte-equivalence + `cluster.<name>.upstream_cx_total == 1` (one lazy dial) + `upstream_rq_total == 2` + `redis.<sp>.downstream_rq_total` increments.
3. **deliberate-break liveness proof (R6)** — recorded in driver comments + README + PROGRESS.md; run with `-count=1` (the cross-side `StatsAsserter` + the downstream-response byte-equivalence are the load-bearing proofs — prove each asserted counter + each response-byte assertion LIVE; e.g. temporarily asserting `downstream_rq_total == 99` MUST fail on both runner paths).

(Per the cross-side-XOR-boot-reject constraint, all cross-side arms share this ONE dir; 32.1 lands arms 1–2, 32.2 extends with the command matrix [GET-miss `$-1`, INCR `:1`, AUTH-no-password error] + the splitter arms [unsupported / bad-arity].)

#### 8.1.1 Driver wire-byte crafting + the byte-equivalence prong

The driver hand-crafts RESP request bytes (inline + array-of-bulk; small builders `respArray(...)`, `respBulk(...)`, `inline(...)` — readable + reusable by the 32.2 command-matrix arms; D-S32.1-4) and CAPTURES the raw reply bytes from BOTH sides, asserting them byte-identical (the byte-equivalence prong) IN ADDITION to the stat parity. Each arm drives both sides identically.

#### 8.1.2 StatsAsserter mechanics — FLAT admin `/stats`, NOT `/stats/prometheus` (§1.2 pin)

The driver implements `fixture.StatsAsserter` (`AssertStats(t, refAdminAddr, subjAdminAddr)`; the scrape-and-diff is IN-BAND — the driver holds both admin addrs). At 32.1 it scrapes the FLAT admin `/stats` text (lines `redis.<sp>.<leaf>: <v>` + `cluster.<name>.<leaf>: <v>`), comparing by INTERNAL NAME — which the reference AND envoy-go expose identically. The `redis.` Prometheus tag-extractor arm (`internal/stats/name.go`) is NOT needed at 32.1 (it lands at 32.2 with the `/stats/prometheus` label-aware comparison upgrade + the dynamic command names). This is the cleanest 32.1↔32.2 boundary (the mongo 29.1 label-aware-scrape pin, in reverse). The exact flat-`/stats` parse helper (a `name → value` map per side over the admin text; the assertion keyed by `redis.<sp>.`/`cluster.<name>.` names) is D-S32.1-5 (IMPL); if the harness lacks a flat-`/stats` asserter helper, a small one is added at 32.1 (the in-band-scrape discipline gives the driver full latitude — `fixture.go:70-77`).

### 8.2 `0056-redis-boot-reject` (32.1; boot-reject; separate dir)

Missing `stat_prefix` → both sides reject at boot (the §6.1 `redis-proxy-stat-prefix-required` arm; boot-stderr-substring parity per §6 — substring `stat_prefix`). Driver implements `fixture.Driver` + `differential.BootRejectFixture` (`harness.go` BootRejectFixture; `BootRejectScript() ""`, `ExpectedBootErrorSubstring() "stat_prefix"`). Symmetric mode; a minimal unused cluster satisfies the zero-cluster boot reject (`reference_network_filter_typeurl_extensions`). The `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` arms are unit-tested at 32.1 (§6.2), NOT fixture arms (D-P32-5 — the `0047`/`0050`/`0054` precedent).

### 8.3 New BackendKind (`TCPRedisResponder`, value 32)

A NEW BackendKind — a synthesized `TCPRedisResponder` (next-free value 32, after `TCPKafkaResponder = 31`; `fixture.go:542`) speaking minimal RESP: it reads RESP request frames and returns canned replies for the exercised data commands (`+OK\r\n` for SET, `$<n>\r\n<val>\r\n` bulk for GET-hit; the 32.2 matrix adds `$-1\r\n` GET-miss + `:<n>\r\n` INCR). FIFO/positional — NO correlation id (contrast `TCPKafkaResponder`'s correlation-id echo; the silent `TCPSink`). PING/AUTH NEVER reach the backend (local-reply, AMEND-R5) — the responder need not handle them. The exact canned-reply table is pinned at the 32.1 IMPL.

### 8.4 Total fixture-dir count + conformance

56 → **58** at 32.1 phase-done (+2: `0055` cross-side [extended at 32.2], `0056` boot-reject). The full 56-dir existing suite is the back-compat regression gate (§4 — the seam is ADDITIVE; it activates only for a redis_proxy terminal) and re-runs byte-exact green at the six-gate. No new conformance harness (matches 26/27/28/29/31); h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected (phase 32 touches no HTTP/h2/proxy-wasm path).

---

## 9. Behavior-contract delta (the 32.1 bundle; per ADR-0052 atomic landing)

ONE atomic bundle at the 32.1 IMPL final task:

- NEW `### envoy.filters.network.redis_proxy` subsection (after the kafka_broker subsection): the terminal-routing-proxy semantics (`Handle` owns the conn; no `tcp_proxy` behind it); the RESP codec value-type framing + inline + null sentinels; the streaming-reader partial-frame model (D-P32-4; contrasted with the observer private-buffer model); the `catch_all` single-cluster routing + the unknown-cluster tolerance (vs tcp_proxy's reject); the PING/AUTH local-reply set (zero upstream); the config parse + the PGV/runtime reject arms (§6); the 10 fixed downstream cx/rq counters + the 4 created-not-yet-incremented gauges; the upstream traffic stats via the reused `cluster.<name>.*` roster.
- NEW `## Network filters — upstream connection-pool / cluster-routing seam (32.1)` framework subsection (the ADR-0230 seam): the API + the one-conn-per-downstream multiplexing model + the synchronous single-flight FIFO/positional correlation contract + the redis-scoped boundary + the deferred shared-pool/deep-queue (§4.4).
- Departure / coverage-boundary records (32.1 subset, §7.5): the boot-window eager-creation difference (unobservable); `op_timeout` parsed-not-consumed; the gauges created-not-incremented (closed at 32.2); runtime-keys-at-defaults; the close-direction-zero-touch posture (AMEND-R9); the per-side one-conn-per-downstream pooling divergence forward-pointer (D-P32-9, pinned at 32.2); forward-pointers to the 32.2 bundle (the full command set + the per-command/splitter/REDIS_CLUSTER_STATS roster + the gauges' inc/dec + the `redis.` Prometheus arm + the histogram/faults coverage boundaries).
- Stat table: 536 → **546** (the 10 new rows).

---

## 10. ADR anchor map (1 §Context anchored at THIS SPEC commit)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL) + the BRAINSTORM §7 / parent §10 locked numbering. At THIS 32.1-SPEC commit, the **ADR-0230 §Context draft** is appended to DECISIONS.md (tail ADR-0229 → ADR-0230; next-free → ADR-0231). This is the deferred half of the phase-32 ADR plan (the parent SPEC anchored ADR-0229 §Context; ADR-0230's §Context anchors HERE because its exact API was a 32.1-SPEC pin — D-P32-3). ADR-0230's §Decision/§Consequences body lands at the 32.1 IMPL per ADR-0044. The ADR-0229 §Decision/§Consequences body lands across the 32.1/32.2 IMPL.

- **ADR-0230** *(32.1 — the upstream connection-pool / cluster-routing seam; §Context HERE; §Decision/§Consequences at the 32.1 IMPL)* — the framework's SIXTH structural extension, in the existing `internal/filter/network/` package: a `TerminalFilter` resolves a routed cluster IN-FILTER (catch_all → `cluster.Manager.Get`, lazily + unknown-tolerant) and round-trips request↔reply through a per-downstream-connection upstream conn the seam manages — dialed via a closure over the as-built `cluster.Cluster.Dial` (keeping `internal/filter/network` free of an `internal/cluster` import; the upstreamcluster.go decoupling discipline), one-conn-per-downstream (AMEND-R8), synchronous single-flight with FIFO/positional pop-front reply correlation (RESP carries no correlation id). Redis-scoped (no thrift generalization; the shared per-host pool + the deep pending queue + the two-goroutine pipelined model + the ADR-0223 per-conn mutex DEFERRED — §4.4). Builds on `TerminalFilter.Handle` (UNCHANGED); ADDITIVE (existing fixtures byte-identical). The seam ADR (the analogue of ADR-0221 [WriteFilter seam] / ADR-0226 [async halt/resume seam]).

The ADR-0229 §Context (the redis_proxy filter umbrella) was anchored at the parent SPEC (DECISIONS.md). Next-free after the 32.1 SPEC = **ADR-0231**; after phase-32 phase-done ≈ ADR-0231 (no further ADR anticipated; each sub-phase re-checks).

---

## 11. SPEC-time empirical-pin block

The 32.1 SPEC does NOT re-execute the parent §11 D32-1..D32-8 pins (resolved once at the parent SPEC live against the contrib reference Envoy; inherited — §1.1). It runs NO new docker probes (the parent §11 block is the authoritative empirical record). The only in-session verification was the as-built source-anchor re-pin below (the §3/§4 design anchors).

### 11.1 As-built anchors VERIFIED at this SPEC session (master tip `617d8c3`)

The source of the §14 Task-1 first-action gate; the IMPL Task-1 RE-RUNS the counts against the live IMPL-session tip.

- **Differential fixture-dir count = 56**; numbering tail = `0054-kafka-boot-reject` (`ls -d test/fixtures/[0-9]* | wc -l`). 32.1 lands `0055` + `0056` → **58**.
- **Fuzzer count = 40** (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l`; tail `FuzzKafkaDecode`). UNCHANGED at 32.1 (the 41st `FuzzRESPDecode` is 32.2).
- **Stat surface = 536** (BEHAVIOR_CONTRACT stat table). 32.1 lands +10 → **546**.
- **BackendKind tail = 31** (`TCPKafkaResponder`, `fixture.go:542`). 32.1 lands `TCPRedisResponder = 32`.
- **DECISIONS.md tail = ADR-0229** (the redis_proxy filter §Context from the parent SPEC); **next-free ADR-0230**. 32.1 SPEC anchors the ADR-0230 §Context (§10); 32.1 IMPL fills the ADR-0230 §Decision/§Consequences body in place (no new number).
- **go-control-plane `/envoy` v1.32.4 redis_proxy bindings present**: `~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/network/redis_proxy/v3/` (redis_proxy.pb.go + .pb.validate.go + _vtproto.pb.go); CORE `/envoy` (AMEND-R1; ZERO new dep).
- **As-built anchors re-verified** (the §3/§4 design anchors): `internal/filter/network/terminal.go:30-49` (`TerminalFilter.Handle`); `internal/filter/network/upstreamcluster.go` (the ADR-0219 override-seam context-channel shape — the decoupling precedent §4.2 preserves); `internal/cluster/manager.go:110-114` (`Manager.Get`) + `:97-108` (`registerClusterMetrics` — the upstream traffic-stat roster, AMEND-R6); `internal/cluster/cluster.go:198-223` (`Cluster.Dial` — Incs upstream_cx_total/active, returns `*connWithGauge`) + `:134` (`IncUpstreamRqTotal`); `internal/filter/tcpproxy/filter.go:49-73,101-159` (`NewFilter` cluster-resolve-and-REJECT [the unknown-cluster contrast, §3.3] + `Handle` terminal dial + pump precedent); `internal/filter/network/builtins/builtins.go:43-83` (the 9 registrations; kafkabroker at `:82`; doc "nine built-in network filters" at `:1`; `tcpproxy.NewNetworkFactory(deps.ClusterManager, …)` the cluster-capture precedent at `:52`); `internal/bootstrap/bootstrap.go` (the network-filter blank-imports; NO redis_proxy import exists); `internal/filter/network/kafkabroker/`+`mongoproxy/` (the §9 package-shape precedents); `internal/stats/registry.go` (`NewCounterIfAbsent`/`NewGaugeIfAbsent`, post-Freeze-permitted) + `IsValidName` (the §3.3 prefix guard); `internal/stats/name.go` (UNTOUCHED at 32.1 — the `redis.` arm is 32.2).
- **Differential-harness anchors**: `test/differential/fixture/fixture.go:70-77` (`StatsAsserter` — in-band scrape, §8.1.2) + the BackendKind roster (`TCPKafkaResponder = 31`, next-free **32**); `harness.go` `BootRejectFixture` (the `0056` shape); the `0053`/`0054` kafka fixtures (the cross-side + boot-reject precedents).

---

## 12. SPEC-time D-questions — parent resolutions + 32.1-additive PLAN/IMPL questions

### 12.1 Parent D-questions RESOLVED at this SPEC

- **D-P32-1 (fixed-roster creation posture) — RESOLVED: EAGER at config parse** (§3.4/§7.2). The boot-window difference vs upstream's per-connection creation is a BEHAVIOR_CONTRACT departure, unobservable to the differential (post-connection assertions).
- **D-P32-3 (the seam API + multiplexing model) — RESOLVED** (§4.2): one-upstream-conn-per-downstream-conn; lazy dial on the first proxied command; synchronous single-flight (depth-1 FIFO / positional pop-front, R4); the seam consumes a DIAL CLOSURE (no `internal/cluster` import in `internal/filter/network`); the shared pool + deep queue + two-goroutine pipelined model deferred (§4.4). The ADR-0230 §Context anchors here (§10).
- **D-P32-4 (RESP partial-frame reassembly) — RESOLVED: the streaming-reader model** (§3.5) — the terminal owns the conn → block-read a `bufio.Reader`, no private high-water buffer (contrasted with the mongo/kafka observer model).
- **D-P32-5 (boot-reject fixture arms) — RESOLVED: `0056` = `stat_prefix` arm ONLY** (§6.2/§8.2); the `settings`/`op_timeout`/`no-upstream`/`catch-all-cluster` arms unit-tested (the kafka `0054`/mongo `0050` precedent).
- **D-P32-6 (32.1 local-reply subset) — RESOLVED: PING + AUTH** (§3.6); ECHO/TIME/QUIT/HELLO are 32.2 follow-ons (the FULL extent stays the 32.2-SPEC D-P32-6).

(Parent D-P32-2 [the per-command roster + the mirrored-vs-per-side upstream counter set], D-P32-7 [IsValidName disposition], D-P32-8 [the gauges' differential assertion design], D-P32-9 [the upstream-pool per-side asymmetry] are 32.2-owned and untouched here, beyond the §4.4 / §7.4 forward-pointers.)

### 12.2 32.1-additive D-questions for PLAN / IMPL resolution

- **D-S32.1-1 (the seam's exact exported signatures).** Finalize `UpstreamConn`/`UpstreamDialFunc`/`Send`/`Reader`/`Close` (or a refined shape — e.g. a single `RoundTrip`-style method that writes + returns the reader). **Resolution at:** IMPL (the seam task). The §4.2 SHAPE + contract (one-conn-per-downstream, lazy dial, single-flight FIFO, dial-closure decoupling) are LOAD-BEARING; the exact names are not.
- **D-S32.1-2 (the RESP decoder internal representation).** The `decodeRequest`/`decodeReply` return shapes (raw-bytes-verbatim anticipated, §3.5) + how the bulk-length / array-count bounds are checked against an overflow cap. **Resolution at:** PLAN / IMPL (the codec tasks).
- **D-S32.1-3 (PARSE-REJECT byte-stable wording).** Finalize the §6 arm wording + the `TestParseRejectConstants_ByteStable` table. **Resolution at:** IMPL (the config task). Anticipated prefix: `redis_proxy: `.
- **D-S32.1-4 (RESP request-byte builder helper shape).** The `0055` driver's request builders (`respArray(...)`, `respBulk(...)`, `inline(...)`), shared with the 32.2 command-matrix arms. **Resolution at:** IMPL (the `0055` task).
- **D-S32.1-5 (flat-`/stats` StatsAsserter mechanics).** Whether the harness already exposes a flat admin `/stats` scrape-and-diff helper or the `0055` driver adds a small `name → value` parser; the assertion keyed by `redis.<sp>.`/`cluster.<name>.` names (§8.1.2). **Resolution at:** IMPL (the `0055` task). Anticipated: the driver parses the admin `/stats` text in-band (the `fixture.go:70-77` in-band discipline gives full latitude).
- **D-S32.1-6 (the lazy-dial guard + the unknown-catch_all-cluster failure path).** How `Handle` ensures-dialed-once on the first proxied command + how a missing cluster fails gracefully (close vs a future `-ERR`). **Resolution at:** IMPL (the filter task). Anticipated: close the downstream connection on a missing cluster (no `-ERR` synthesized at 32.1 — a coverage-boundary note).

---

## 13. RATIFIED-PENDING items (cross-reference parent §13, scoped to 32.1)

- **R1 (seam back-compat).** The upstream-pool seam is ADDITIVE (it activates only for a redis_proxy terminal); the 56 existing fixture dirs (`0000`..`0054`) stay byte-exact green at the 32.1 six-gate (redisproxy registered but unconfigured in them — the registration-perturbs-nothing gate). `TerminalFilter.Handle`/`tcp_proxy`/HCM untouched.
- **R2 (roster + scope parity — 32.1 subset).** The 10 fixed names under `redis.<stat_prefix>.` match upstream name-for-name. Ratified by `TestStatRoster32_1_MatchesUpstream` + the `0055` post-connection assertions. (The full-15 + per-command roster test is 32.2's.)
- **R3 (downstream-response byte-equivalence — the §9 FIRST).** The RESP bytes redis_proxy returns are byte-identical cross-side for the `0055` 32.1 arms (PING `+PONG`, SET `+OK`, GET-hit bulk). Ratified by the `0055` response-byte assertion + the deliberate-break liveness proof.
- **R4 (FIFO/positional correlation).** Replies dequeue positionally (the synchronous single-flight pump = depth-1 FIFO); the single-route single-key envelope keeps it trivially in-order. Ratified by the `0055` SET-then-GET round-trip + a unit/`-race` seam test.
- **R5 (PING/AUTH local-reply; zero upstream).** PING/AUTH answered in-filter with zero upstream traffic (AMEND-R5); they count `downstream_rq_total` but never dial. Ratified by the `0055` PING arm (`cluster.<name>.upstream_cx_total`/`upstream_rq_total` asserted 0).
- **R6 (StatsAsserter liveness).** Every `0055` stat assertion proven live via a recorded deliberate-break with `-count=1` (`reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1`).
- **R8 (zero new dep).** redis_proxy/v3 is core `/envoy v1.32.4` → `go mod tidy` adds nothing with the first consumer; the `@type` pinned via `proto.MessageName`. Ratified by the IMPL Task-1 pinning test + a clean `go mod tidy`.
- **R9 (counts re-pinned).** IMPL Task 1 re-pins fixtures 56→58, fuzzers 40 (unchanged), stat surface 536→546, BackendKind tail 31→32, DECISIONS.md tail ADR-0230 (the §Context anchored at THIS SPEC; the body fills at IMPL) against the live IMPL-session tip.

(Parent R7 [Prometheus parity] is 32.2's — the `redis.` arm + the `/stats/prometheus` comparison land at 32.2.)

---

## 14. Per-task structure (~16 tasks; the SPEC-anticipated task spine)

The 32.1 PLAN authors the exact bite-sized TDD tasks (the PLAN may merge/split); this is the SPEC-anchored spine:

| # | Task | Lands |
|---|---|---|
| 1 | First-action baselines/anchors gate: re-pin fixtures **56** (tail `0054`) + fuzzers **40** + stat surface **536** + BackendKind tail **31** + DECISIONS tail **ADR-0229** (next-free **ADR-0230**, the §Context anchored at this SPEC) + the `proto.MessageName` TypeURL pinning test + a clean `go mod tidy` (ZERO new dep, R8) + the §11.1 as-built anchors, against the live IMPL-session tip | §11 / R8 / R9 |
| 2 | `redisproxy` package skeleton + `TypeURL` (proto.MessageName) + `NewFactory(cm, reg)` + `doc.go` | §3.2 |
| 3 | `config.go` parse (`stat_prefix` + `IsValidName` guard + `settings`/`op_timeout` + `catch_all_route.cluster` PGV arms + the runtime no-upstream check + the unknown-cluster tolerance + deferred-field parse-accept) + parse unit tests | §3.3 |
| 4 | PARSE-REJECT constants (`redis_proxy: ` arms) + `TestParseRejectConstants_ByteStable` (the §6.2 arms unit-armed) | §6 |
| 5 | `resp.go` part 1: request decode (inline + array-of-bulk) + value-type framing + the streaming-reader partial-frame model + malformed/overflow handling + no-panic unit tests | §3.5 |
| 6 | `resp.go` part 2: reply decode (all 5 value types + null sentinels + nested arrays; raw-bytes verbatim) + encode (PING `+PONG` / AUTH error) + unit tests | §3.5 |
| 7 | `upstreampool.go`: the seam (`UpstreamDialFunc` + `UpstreamConn` + lazy dial + single-flight `Send`/`Reader` + `Close`) + unit + `-race` round-trip test (no `internal/cluster` import in `internal/filter/network`) | §4.2 |
| 8 | `commands.go`: the 32.1 local-reply set (PING / AUTH) + the local-vs-proxy dispatch + unit tests | §3.6 |
| 9 | `stats.go`: the 10 eager names (6 counters + 4 gauges) + `TestStatRoster32_1_MatchesUpstream` + the 32.1 inc accessors | §3.4 / §7.2 / R2 |
| 10 | `filter.go`: the `TerminalFilter.Handle` pump (decode → PING/AUTH local-reply OR lazy-resolve catch_all + seam round-trip → write reply) + the downstream cx/rq + byte-count lifecycle + unit tests (single command; pipelined SET-then-GET; unknown-cluster graceful-close; EOF clean-close) | §3.7 |
| 11 | The 10th built-in registration + `bootstrap.go` blank-import + a boot smoke test (a `[redis_proxy]` terminal bootstrap boots; the 10 stats exist at 0; clean `go mod tidy`) | §3.8 |
| 12 | `TCPRedisResponder` BackendKind (value 32) + the canned-reply RESP responder (`+OK` SET / `$<n>` GET-hit) | §8.3 |
| 13 | `0055` driver part 1: bootstraps (both sides; `[redis_proxy]` terminal; catch_all → `TCPRedisResponder`) + the RESP request builders + the PING arm (local-reply; `+PONG` byte-equivalence; upstream 0) | §8.1 / §8.1.1 |
| 14 | `0055` driver part 2: the proxied SET/GET round-trip arm (downstream-RESP byte-equivalence + `cluster.<name>.upstream_cx/rq` + downstream cx/rq) + the flat-`/stats` StatsAsserter + the deliberate-break liveness recording (R6) | §8.1.2 / R3 / R6 |
| 15 | `0056-redis-boot-reject` fixture (the `stat_prefix` arm; symmetric boot-reject) | §8.2 |
| 16 | Completion bundle: ADR-0230 §Decision/§Consequences body in-place (ADR-0044) + the BEHAVIOR_CONTRACT 32.1 bundle (§9) + STATE.md + ROADMAP sub-row 32.1 `in-progress → done` (parent row 32 STAYS `in-progress`) + next-prompt.txt (the 32.2-SPEC cold-start) + the six-gate (incl. the FULL 58-dir differential suite + the 56-dir back-compat gate) | §9 / §16 |

### 14.1 ADR-0045 split-gate — SPEC-level re-check (parent §15 row "32.1")

Production-LoC estimate against the §3/§4/§7 refined surface (the 26.x–31 accounting basis: production code; fixture drivers + unit tests EXCLUDED):

| Deliverable | Production LoC |
|---|---|
| `config.go` (PGV arms + runtime check + unknown-cluster tolerance + IsValidName guard + parse-accept) | ~120–170 |
| `resp.go` (value-type framing + request/reply decode + encode + streaming reassembly + overflow guards) | ~300–420 |
| `upstreampool.go` (the seam: dial closure + lazy dial + single-flight Send/Reader + Close + lifecycle) | ~120–200 |
| `commands.go` (PING/AUTH local-reply + dispatch) | ~60–110 |
| `stats.go` (the 10-name roster + inc accessors) | ~70–110 |
| `filter.go` + `redisproxy.go` + `doc.go` (the Handle pump + factory + glue) | ~180–260 |
| builtins + bootstrap.go (the 10th built-in + blank-import) | ~30–50 |
| `TCPRedisResponder` BackendKind (the canned RESP responder) | ~60–100 |
| **Total (production basis)** | **~940–1420** |

**Verdict: fits as ONE sub-phase** (under the ~1500 LoC gate; the ~16-task spine under the ~25-task gate). The fixture drivers (`0055`+`0056`; the `0053`/`0054` kafka precedents ~600–900 across both) are excluded per the 26.x–31 accounting precedent. **The 32.1 PLAN remains the FINAL gate-check** (parent §3.0): if the bite-sized TDD decomposition exceeds ~25 tasks, the pre-authorized escape-valve split axis is **32.1a** (Tasks 1–11: the package + the seam + the codec + builtins/bootstrap — a bootable, unit-proven redis_proxy) / **32.1b** (Tasks 12–16: the `TCPRedisResponder` + `0055` + `0056` + the completion bundle — its cross-side differential proof). The 2-way pre-split holds at SPEC time.

---

## 15. (reserved)

(No separate section — the LoC/split-gate re-check is §14.1; the test surface is §16.)

---

## 16. Test surface + 32.1 IMPL acceptance checklist

### 16.1 Test surface (per parent §14, scoped to 32.1)

- **Layer A — redisproxy unit tests**: config parse (all PGV arms incl. `settings`/`op_timeout`/`catch_all_route.cluster`; the runtime no-upstream reject; the unknown-cluster TOLERANCE [boots clean]; the `IsValidName(stat_prefix)` guard; the deferred-field parse-accept); the RESP codec (each value type + null sentinels + inline + nested arrays; partial-frame reassembly across reads [block-and-resume]; malformed/overflow/truncated frames → error-not-panic; raw-bytes-verbatim forwarding); the `TerminalFilter.Handle` pump (PING/AUTH local-reply; a single proxied command; a pipelined SET-then-GET on one connection [the FIFO/positional proof]; unknown-cluster graceful close; EOF clean close); the downstream cx/rq + byte-count increments.
- **Layer A — seam unit tests** (`internal/filter/network/`): `UpstreamConn` host-dial-via-closure (lazy on first `Send`; no dial on a PING-only path); the single-flight `Send`/`Reader`/`Close` round-trip; a `-race` test (the `Handle` goroutine + the seam's per-connection state); a build-level assertion that `internal/filter/network` gains NO `internal/cluster` import.
- **Layer D — differential**: `0055` (cross-side byte-equivalence + flat-`/stats` StatsAsserter; PING + proxied round-trip arms) + `0056` (boot-reject) + the FULL 56-dir back-compat suite (R1) → 58/58 green.
- **Layer E — race**: `go test ./... -race -short` across `internal/filter/network/...` + `internal/filter/network/redisproxy/...`.
- Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).

(No fuzzer at 32.1 — `FuzzRESPDecode` is the 41st, 32.2; §2.10. The 32.1 no-panic guarantee is unit-test-proven.)

### 16.2 Six-gate checklist (per the 26–31 precedent)

`go build ./...` / `go vet ./...` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact (58 dirs incl. the 56-dir back-compat gate) / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 32.1 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly).

### 16.3 32.1 IMPL acceptance checklist

1. The `redisproxy` package lands per §3 (config parse + the RESP codec + the `Handle` pump + the 10-name eager roster + the PING/AUTH local-reply + the lazy catch_all resolution); the upstream-pool seam lands in `internal/filter/network/` per §4 (one-conn-per-downstream; synchronous single-flight; dial-closure decoupling — no `internal/cluster` import in the core package).
2. The 10th built-in + `bootstrap.go` blank-import land (§3.8); a clean `go mod tidy` adds ZERO modules (R8).
3. Fixtures `0055` (byte-equivalence + flat-`/stats` StatsAsserter; PING + proxied arms) + `0056` (boot-reject) green; the `TCPRedisResponder` BackendKind (32) lands; counts: fixtures 56→58, fuzzers 40 (unchanged), stats 536→546, BackendKind 31→32 (R9).
4. ADR-0230 §Decision/§Consequences body lands in place (DECISIONS.md tail STAYS ADR-0230; no new number consumed); the BEHAVIOR_CONTRACT 32.1 bundle lands (§9).
5. Six gates green (§16.2); STATE.md advanced; ROADMAP sub-row 32.1 `in-progress → done`; parent row 32 STAYS `in-progress` (the ROLLUP is 32.2's); next-prompt.txt rewritten for the 32.2-SPEC cold-start.

---

## 17. Stage-close handoff

Per ADR-0004/0005: this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, ROADMAP sub-row 32.1 flips **`planned → in-progress` AT THIS SPEC COMMIT** (ADR-0106 / the 26.x/28.x/29.x precedent); parent row 32 STAYS `in-progress`; 32.2 STAYS `planned`. ALSO at this commit: the **ADR-0230 §Context** is appended to DECISIONS.md (tail ADR-0229 → ADR-0230; §10). STATE.md advances to lifecycle-state-for-32.1-PLAN with `next-skill = superpowers:writing-plans` scoped to the **32.1 PLAN** (`docs/envoy-go/phases/32.1-network-filter-redis-upstream-pool-seam-and-codec/PLAN.md`). The SPEC + the ADR-0230 §Context are squash-merged to master + pushed (`feedback_push_to_origin`; the controller squash-merges + pushes at stage-close, subagents local-only per `feedback_subagents_no_push`); next-prompt.txt is rewritten for the 32.1-PLAN cold-start. Per `feedback_execution_style` the 32.1 IMPL runs `superpowers:subagent-driven-development`; per `feedback_git_worktrees`/`feedback_subagents_no_push`/`feedback_push_to_origin`/`feedback_pertask_gofmt_lint` the established worktree/push/lint discipline applies.

---

## Appendix A — Cross-references to parent SPEC

| 32.1 SPEC § | Parent SPEC § | Relationship |
|---|---|---|
| §1 Purpose | parent §1 + §3.2 (32.1 detail) | refines |
| §1.1 AMENDs | parent §1.1 (R1/R2/R5/R6/R8/R9) | inherits the 32.1-load-bearing subset |
| §1.2 Additive pins | — | NEW (the D-P32-1/3/4/5/6 resolutions; the unknown-cluster tolerance; the flat-`/stats` + byte-equivalence prongs) |
| §2 Non-purposes | parent §2 + §3.2 | refines (32.1-scoped) |
| §3 redisproxy package | parent §4.2 + §11.5/§11.6 | refines into the file split + production shapes |
| §4 the seam | parent §4.1 + §11.3 (AMEND-R8) | PINS the API + multiplexing model (D-P32-3); ANCHORS ADR-0230 §Context |
| §5 Proto roster | parent §5 | INHERITS verbatim |
| §6 PARSE-REJECT | parent §6 | refines (32.1 arms; wording at IMPL); resolves D-P32-5 |
| §7 Stat surface | parent §7 | refines (the 32.1 +10 subset); resolves D-P32-1 |
| §8 Fixtures | parent §8.1/§8.2/§8.3 | refines (32.1 arms + TCPRedisResponder + flat-`/stats` + byte-equivalence) |
| §9 Behavior contract | parent §9 (32.1 bundle) | refines |
| §10 ADR anchor map | parent §10 (the deferred ADR-0230 §Context) | ANCHORS ADR-0230 §Context |
| §11 Empirical pins | parent §11 (inherited; no re-probe) + the as-built re-pin | inherits |
| §12 D-questions | parent §12 | resolves D-P32-1/3/4/5/6; adds D-S32.1-1..6 |
| §13 RATIFIED-PENDING | parent §13 (R1–R9) | scoped to 32.1 |
| §14 Tasks + split-gate | parent §15 (32.1 row) | NEW (task spine); gate re-check |

## Appendix B — Phase 32.1 ADR landing summary

- **ADR-0230** (the upstream connection-pool / cluster-routing seam) — §Context anchored at THIS 32.1 SPEC (DECISIONS.md tail ADR-0229 → ADR-0230); §Decision + §Consequences bodies land at the 32.1 IMPL final task per ADR-0044. This SPEC's §4 is the body's blueprint: the as-built anchors (§4.1), the API + one-conn-per-downstream multiplexing model + the dial-closure decoupling (§4.2), the in-filter routing (§4.3), the deferred shared-pool/deep-queue boundary (§4.4).
- **ADR-0229** (the `redis_proxy` filter + parent-row-32 umbrella) — §Context anchored at the parent SPEC; §Decision/§Consequences bodies land across the 32.1 IMPL (the filter + codec + seam consumption + the 10 fixed counters + PING/AUTH) + the 32.2 IMPL (the full command set + the per-command/splitter/REDIS_CLUSTER_STATS roster + the gauges + the `redis.` Prometheus arm + the rollup).
- DECISIONS.md tail = **ADR-0230** at the 32.1 SPEC commit (its §Context); next-free after phase-32 phase-done ≈ **ADR-0231**.
