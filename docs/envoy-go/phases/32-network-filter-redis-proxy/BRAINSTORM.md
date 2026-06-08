# Phase 32 Brainstorm — `redis_proxy` (SIXTH §9 Network-filters-family row; the project's FIRST terminal routing proxy)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 32 (`network-filter-redis-proxy`), the **SIXTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, the phase-28 `zookeeper_proxy` parent row, the phase-29 `mongo_proxy` parent row, and the phase-31 `kafka_broker` flat row; the phase-30 contrib pin-refresh was an infra row, not a family member). Phase 32 lands `envoy.filters.network.redis_proxy` — the project's **FIRST terminal routing proxy**. Unlike every prior §9 row (echo / sni_cluster / zookeeper_proxy / mongo_proxy / kafka_broker — observational sniffers inserted before `tcp_proxy` on a `[filter, tcp_proxy]` chain), redis_proxy IS the connection terminator: it parses RESP, **routes** commands to an upstream cluster, **pools/pipelines** upstream connections itself, and writes replies back downstream. There is no `tcp_proxy` behind it, so there is **no passive-observe MVP** — the filter must actually serve traffic to be a valid Envoy config. The load-bearing new capability is therefore a framework first: a terminal that routes to an upstream cluster and pools/pipelines upstream connections — neither exists at master tip.

The next sessions author the parent SPEC then the per-sub-phase SPEC/PLAN/IMPL cycles (lifecycle-state 1 → 2 for phase 32, skill `superpowers:writing-plans` scoped to **parent SPEC authoring** per the phase 22 / 25 / 26 / 28 / 29 parent-row precedent). The parent SPEC formalizes the 2-way split surface-mapping + executes the §10 empirical-pin obligations (D32-1..D32-8) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0229 §Context draft. The per-sub-phase SPEC sessions (32.1 / 32.2) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the 22.1 / 25.1 / 26.1 / 28.1 / 29.1 precedent.

**Brainstorm session:** worktree `.worktrees/phase-32-network-filter-redis-proxy-brainstorm`, branch `phase-32-network-filter-redis-proxy-brainstorm`. Substantive predecessor on master: the phase-31 IMPL squash `928287a` (the `kafka_broker` filter — the FIFTH §9 row; ADR-0228; stat surface 360 → 536; fixtures 54 → 56; fuzzers 39 → 40; BackendKind tail 30 → 31; docs-only SHA-fill follow-ups trail it as the live tip).

**Brainstorm mode:** interactive with a live human. The user picked the subject + each major design decision via a multi-question dialogue:

- **Q0 subject selection** — `redis_proxy` (`envoy.filters.network.redis_proxy`), chosen over `thrift_proxy` from the two remaining §9 candidates {redis, thrift} (both terminal-proxy surfaces). Rationale (user-confirmed): RESP is dramatically simpler than Thrift's transport-matrix (framed/unframed/header) × protocol-matrix (binary/compact/twitter) + method-routing + nested thrift-filter sub-chain; redis routing is key/prefix-based and the leaner of the two terminal proxies. After phase 32 the remaining §9 candidate is **{thrift}**.
- **Q1 scope envelope** — `Single-route terminal` (over {Routing-complete (prefix_routes multi-cluster) / Narrowest decode+round-trip}). RESP codec (simple-string / error / integer / bulk-string / array + inline commands) + `catch_all_route` to ONE upstream cluster + a basic per-host upstream connection pool with pipelined request↔reply correlation + the core single-key command set + per-command/downstream/upstream stats. DEFER (parse-accept-silent-ignore vs parse-reject decided empirically at SPEC): `prefix_routes` beyond catch_all, hash-ring sharding + `enable_redirection`/MOVED-ASK, multi-key command fragmentation (MGET/MSET/DEL split-collate), `downstream_auth_*`, `faults`, request mirroring, `external_auth_provider`, replica `read_policy`, `custom_commands`, `ConnPoolSettings` timeouts/flush-batching. The leanest slice that is still a real, differential-testable proxy.
- **Q-split sizing** — `Pre-split into 2, seam-first` (over {pre-split into 3 / single flat row 32}). Parent row 32 + sub-phases 32.1 (the upstream connection-pool/routing FRAMEWORK SEAM + the RESP codec + a minimal `catch_all` single-command round-trip proven end-to-end via differential — seam-first, like 28.1a/29.3) and 32.2 (the full single-route command set + the per-command/downstream/upstream stat roster + the differential command matrix + the fuzzer — the consumer). Parent BRAINSTORM + parent SPEC here; each sub-phase its own SPEC → PLAN → IMPL (the mongo-29 shape).
- **Q-pool-seam placement** — `Framework-level, redis-scoped` (over {redis-package-local now, promote later / framework-level designed for both redis+thrift}). The upstream connection-pool / cluster-routing capability lands in `internal/filter/network/` as a genuine framework seam (a terminal filter resolves a cluster host, dials, pools the TCP conn, pipelines request↔reply in FIFO order), but its surface is scoped to EXACTLY what redis needs now — NO thrift-specific generalization (YAGNI). Thrift later reuses/extends it. This is the 32.1 seam-first deliverable and matches the project's seam-first structural-extension discipline (28.1, 29.3).

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0228), and the as-built §9 framework (26.1/26.2/26.3/27/28/29/31). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–31 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/29-network-filter-mongo-proxy/BRAINSTORM.md` section-for-section (the most recent parent pre-split precedent), reframed for the redis_proxy terminal-routing-proxy scope + the 2-way pre-split + the new upstream-pool framework seam. Phase 32 sits in a structurally meaningful position: it is the project's **FIRST terminal routing proxy** (all prior §9 filters were observational sniffers or trivial terminals like echo/direct_response); it lands the framework's **SIXTH structural extension** — the upstream connection-pool / cluster-routing seam (the first seam that lets a network filter ACTIVELY manage upstream connections rather than observe a `tcp_proxy`-owned connection); and it is the second §9 row whose deferred-active-feature surface (sharding/redirection/fragmentation/AUTH/faults/mirroring) materially exceeds its MVP. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-08.

---

## 1. Mission and scope confirmation (32 only)

ROADMAP row `32 | network-filter-redis-proxy | 31 | in-progress | 32.1, 32.2 | …` (added by this brainstorm) is the **parent row** this brainstorm registers as `in-progress` with sub-phase list `32.1, 32.2`. The two sub-rows `32.1 | network-filter-redis-upstream-pool-seam-and-codec | 31 | planned | | …` and `32.2 | network-filter-redis-commands-and-stats | 32.1 | planned | | …` are also registered by this brainstorm (long-prefix slug convention; phase-22/25/26/28/29 precedent). It is a §9 Network-filters-family row split INTERNALLY per ADR-0106(d) (the family flat-row discipline forbids a §9 *umbrella* parent spanning multiple filters, but EXPRESSLY preserves ADR-0045's split-gate WITHIN a single filter-row using the trunk-phase parent+sub-phase pattern — exactly as zookeeper [28.1a/28.1b/28.2] and mongo [29.1/29.2/29.3] split). The parent row's `depends-on` anchor is phase 31 (the kafka_broker §9 row; substantive predecessor `928287a`).

The Network filters family candidate roster at `ROADMAP.md` (§ Feature Families → Network filters family) immediately BEFORE this brainstorm's registration commit was: `redis, thrift` (echo, direct_response, sni_cluster, rbac_network, zookeeper — DONE via phases 26/27/28; mongo — DONE via phase 29; kafka_broker — DONE via phase 31). Phase 32 lands **`redis_proxy`** (this commit updates the roster paragraph to mark redis IN-PROGRESS/landing). After phase 32 phase-done, **1** family candidate remains (`thrift`). Branch/directory/Go-package identifiers: branch `phase-32-network-filter-redis-proxy-brainstorm`, directory `32-network-filter-redis-proxy/`, filter package `internal/filter/network/redisproxy/` (Go package `redisproxy`, single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy`/`kafkabroker` precedent).

Phase 32 is also: (i) the project's **FIRST terminal routing proxy** — it does not sit before `tcp_proxy`; it REPLACES `tcp_proxy` as the connection terminator (via the existing `TerminalFilter.Handle(ctx, conn)` seam used by echo/direct_response) and routes/pools upstream connections itself. (ii) the landing point for the framework's **SIXTH structural extension** — the upstream connection-pool / cluster-routing seam (§3.1), which lets a terminal filter resolve a cluster host, dial, pool the TCP conn, and pipeline request↔reply in FIFO order. Every prior §9 filter either observed a `tcp_proxy`-owned connection (zookeeper/mongo/kafka_broker) or published an override `tcp_proxy` consumed (sni_cluster); NONE managed an upstream connection. (iii) NOT a consumer of the ADR-0221 WriteFilter seam nor the ADR-0226 async halt/resume seam — those govern the read/write *observation* chain on a `tcp_proxy`-terminated connection; a terminal routing proxy owns both ends directly (the upstream-pool seam is its analogue).

### 1.1 What phase 32 delivers as a self-contained whole (envelope: single-route terminal per Q1)

Phase 32 lands `envoy.filters.network.redis_proxy` at a single-route-terminal MVP, across TWO sub-phases:

1. **Sub-phase 32.1** (`network-filter-redis-upstream-pool-seam-and-codec`) — delivers: (a) the **upstream connection-pool / cluster-routing FRAMEWORK SEAM** in `internal/filter/network/` (§3.1; ADR-0230) — a terminal filter resolves a host of a named cluster via the existing `cluster.Manager`, dials a TCP conn (reusing the as-built cluster TCP dial path that `tcp_proxy` uses), pools the conn per upstream host, and offers an in-order (FIFO) request→reply round-trip primitive (RESP has no correlation IDs — replies return in request order per upstream connection, so correlation is positional, NOT id-keyed); redis-scoped (the pool moves bytes + correlates by order; protocol framing stays in the redisproxy package); (b) the **`internal/filter/network/redisproxy/` package foundation** — TypeURL via `proto.MessageName` on the `RedisProxy` proto (the `extensions.` lesson, `reference_network_filter_typeurl_extensions`), config parse of `stat_prefix` + `settings` (ConnPoolSettings, minimum subset) + `prefix_routes.catch_all_route.cluster` (only the catch_all leg consumed; the rest of `prefix_routes` parse-accepted, behavior-deferred), the **RESP codec** (decode downstream request frames + decode upstream reply frames + encode — value types: simple-string `+`, error `-`, integer `:`, bulk-string `$`, array `*`, plus inline commands), and a minimal `catch_all` single-command round-trip (PING/GET/SET) proving the seam end-to-end via a differential fixture; (c) registration as the **10th network-filter built-in** (`builtins.RegisterBuiltins` single insertion) + the `redis_proxy/v3` proto blank-import in `internal/bootstrap/bootstrap.go`; (d) the new BackendKind (a synthesized TCP RESP responder — §6.3); (e) the first differential fixture **`0055-redis-roundtrip`** (cross-side) + the boot-reject fixture **`0056-redis-boot-reject`** if the parent SPEC pins required-field arms there; (f) the BEHAVIOR_CONTRACT 32.1 bundle + STATE/ROADMAP advance.
2. **Sub-phase 32.2** (`network-filter-redis-commands-and-stats`) — delivers: (a) the **full single-route command set** (the core single-key commands routed via catch_all) on top of the 32.1 seam; (b) the **per-command + downstream + upstream stat roster** under `redis.<stat_prefix>.` (the exact roster + eager-vs-dynamic per-command creation pinned empirically at SPEC, D32-2) + the new `redis.` Prometheus tag-extractor arm at `internal/stats/name.go` (the `.zookeeper.`/`.mongo.`/`kafka.` precedent; inline-vs-label-hoist pinned at SPEC), with wire-derived dynamic command-stat segments guarded by `stats.IsValidName` before `NewCounterIfAbsent` (per `reference_dynamic_stat_name_charset_guard`); (c) the **differential command matrix** (a spread of commands + error/edge arms) extending `0055`; (d) the **fuzzer** `FuzzRESPDecode` (the 41st — no-panic / no-mutation / bounded-buffer on the RESP decoder); (e) the BEHAVIOR_CONTRACT 32.2 bundle + the **parent-row-32 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 32.2 per the 18/19/22/24/25/26/28/29 precedent) + the six-gate.

### 1.2 What phase 32 does NOT deliver (forward to §8)

See §8. Highlights: `prefix_routes` multi-cluster routing beyond `catch_all`; hash-ring sharding + `enable_redirection` (MOVED/ASK); multi-key command fragmentation (MGET/MSET/DEL split-and-collate); downstream `AUTH` (`downstream_auth_password`/`downstream_auth_username`/`external_auth_provider`); `faults` (error/delay fault injection); request mirroring (`request_mirror_policy`); replica `read_policy`; `custom_commands`; the full `ConnPoolSettings` surface (op timeouts, flush batching, `max_upstream_unknown_connections`, `connection_rate_limit`); command-latency histograms (ADR-0060); the remaining protocol proxy (`thrift`).

### 1.3 Phase-done as the SIXTH Network-filters-family row landing

After phase 32, the family candidate count drops 2 → **1** (`thrift`). The framework's sixth structural extension (the upstream connection-pool / cluster-routing seam) will have landed seam-first at 32.1, available for thrift to reuse/extend at its phase. The remaining candidate is the heaviest of the original set (`thrift` — a transport×protocol matrix + method-routing + a nested thrift-filter sub-chain) and needs its own brainstorm-time risk assessment.

### 1.4 ADR-0045 split readiness — 2-way pre-split chosen per Q-split

Per ADR-0045 §6 (and ADR-0106(d), which preserves the gate WITHIN a §9 filter-row), the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 32's single-route-terminal surface is anticipated to exceed the LoC leg as a single phase, driven by the brand-new framework seam:

- **RESP codec** (request decoder + reply decoder + encoder; value types + inline) — ~400–600 LoC.
- **The upstream connection-pool / cluster-routing seam** (host resolution + dial + per-host pool + FIFO request↔reply pipelining + lifecycle/teardown) — ~400–700 LoC (the project's SIXTH structural framework extension; seam-first).
- **The redisproxy filter glue** (config parse + the `TerminalFilter.Handle` loop + command→reply pump + stats wiring) — ~300–500 LoC.
- **The stat roster** (per-command + downstream + upstream families + the `redis.` prom arm) — ~100–200 LoC.
- **Differential infra** (the `TCPRedisResponder` BackendKind + the `0055`/`0056` fixtures + the `FuzzRESPDecode` fuzzer) — ~300–500 LoC test.

Anticipated ~1200–2000 production LoC / ~22–32 tasks as a single phase → the LoC leg trips and the task leg is borderline. The project's **seam-first discipline** (zookeeper landed the write/read seams in their own sub-phases 28.1a/28.1b before the response consumer; mongo landed the async halt/resume seam in 29.3 WITH its first consumer) argues for isolating the upstream-pool seam regardless. → the **2-way FEATURE-PROGRESSIVE pre-split at BRAINSTORM time** (the project's SIXTH BRAINSTORM-time pre-split after 22/25/26/28/29). The split axis is consume-at-consumer-ordered: 32.1 = the seam + the RESP codec + a minimal catch_all round-trip (independently shippable: a real single-command redis proxy with a live round-trip + a differential fixture); 32.2 = the full command set + the stat roster + the differential command matrix + the fuzzer (completes the MVP surface). Each sub-phase re-checks the gate at its own PLAN time; a 32.x escape-valve split stays available if either sub-phase trips the gate again.

### 1.5 Seed-stub alignment + package naming

No seed-stub for redis exists. Phase 32.1 creates `internal/filter/network/redisproxy/` from scratch (package `redisproxy`; matches the `directresponse`/`snicluster`/`zookeeperproxy`/`mongoproxy`/`kafkabroker` single-token-joined convention). The RESP codec lives INSIDE the redisproxy package (`resp.go`) — NOT a new top-level package (YAGNI; extract-at-second-consumer). The upstream connection-pool / cluster-routing seam lands IN the existing `internal/filter/network/` framework package (NOT a new package) but is redis-scoped (Q-pool-seam) — its surface is exactly what redis needs; thrift reuses/extends it at its phase.

### 1.6 No prebrainstorm-notes branch

No `phase-32-*-prebrainstorm-notes` branch exists. Phase 32 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 32's relationship to prior framework deltas

Phase 32 continues the framework-delta-GROWTH posture (contrast the framework-zero-touch consumer rows 27/31). Prior lineage (abridged; see 26/28/29 BRAINSTORM §1.7): 07.1 HTTP filter framework → … → 26.1 `internal/filter/network/` read-filter framework → 26.2 `TerminalFilter` seam (`Handle(ctx, conn)`) → 27 connection-scoped upstream-cluster-override (ADR-0219) → 28.1a `network.WriteFilter` seam (ADR-0221) → 28.1b post-handoff read seam (`readChainConn`/`replayRead`) → 29.3 async halt/resume seam (ADR-0226). **Phase 32.1 adds the framework's SIXTH structural extension — the upstream connection-pool / cluster-routing seam** (ADR-0230): the first seam that lets a network filter ACTIVELY manage an upstream connection (resolve a cluster host, dial, pool, pipeline) rather than observe a `tcp_proxy`-owned one. It builds ON the existing `TerminalFilter.Handle` seam (redis_proxy IS a terminal filter) and the existing `cluster.Manager` host-resolution + TCP dial path (the same one `tcp_proxy` uses) — its "newness" is the pool + FIFO pipelining + the routing decision made from inside a filter, NOT a new connection primitive. The deferred sharding/redirection/fragmentation/AUTH/faults/mirroring surface (§8) WOULD each extend this seam (multi-cluster routing, hash-ring host selection, request fan-out/collate), but they are deferred.

---

## 2. Design decisions

### 2.1 Subject selection: `redis_proxy` *(Q0 → phase 32 parent row registered)*

**Decision:** Phase 32 = `envoy.filters.network.redis_proxy` (proto `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy`; the module + import path + package alias are pinned at SPEC, D32-1 — redis_proxy is a CORE extension under `envoy.extensions.filters.network.redis_proxy.v3`, anticipated to live in the EXISTING `github.com/envoyproxy/go-control-plane/envoy v1.32.4` dep [NOT `/contrib`, unlike kafka_broker] → anticipated ZERO new go.mod dep; confirmed at SPEC). Chosen over `thrift_proxy` from the two remaining §9 candidates {redis, thrift}.

**Rationale:** Both remaining §9 candidates are terminal routing proxies (command routing + upstream pooling — a framework first). Of the two, redis is materially leaner: RESP is a simple length-prefixed type system (`+`/`-`/`:`/`$`/`*` + inline commands) decoded with byte scanning only, vs Thrift's transport-matrix (framed/unframed/header) × protocol-matrix (binary/compact/twitter) + method-based routing via a `route_config` + an entire nested thrift-filter sub-chain (router/rate_limit/header_to_metadata). Redis routing is key/prefix-based (catch_all suffices for the MVP); Thrift routing needs the method-routing table from the start. Landing the upstream-pool seam against the simpler protocol first de-risks the framework extension; thrift reuses the seam at its (later) phase.

**Anticipated ADRs:** ADR-0229 (the redis_proxy filter + the parent-row umbrella) + ADR-0230 (the upstream connection-pool / cluster-routing framework seam; see §7).

### 2.2 Scope envelope: single-route terminal *(Q1 → 2-way pre-split; ADR-0229)*

**Decision:** Deliver a single-route-terminal MVP: the **RESP codec** (simple-string / error / integer / bulk-string / array + inline commands), `catch_all_route` to ONE upstream cluster, a basic per-host **upstream connection pool** with pipelined (FIFO) request↔reply correlation, the **core single-key command set**, and per-command / downstream / upstream **stats**. Every deferred field (§1.2 / §8) is either PARSE-ACCEPTED-behavior-DEFERRED or PARSE-REJECTED at config load — the disposition per field is pinned empirically at SPEC (D32-4). The differential mirrors the catch_all-single-cluster behavior exactly.

**Rationale:** Because redis_proxy is the connection TERMINATOR (no `tcp_proxy` behind it), there is no observational MVP — the leanest coherent slice is a real proxy that routes catch_all to one cluster and round-trips RESP. This isolates the load-bearing new framework capability (the upstream-pool seam) with the smallest possible routing surface (one cluster, no sharding, no fragmentation) and command surface (single-key commands). `prefix_routes` multi-cluster routing, hash-ring sharding + redirection, and multi-key fragmentation each ride on top of the seam and add routing complexity for zero additional seam-validation value at the MVP. Wire framing (RESP) is adopted VERBATIM from upstream (`reference_wire_format_both_sides_see_same_bytes` — both the reference Envoy and envoy-go speak the same RESP bytes to the same backend).

### 2.3 The upstream connection-pool / cluster-routing seam: framework-level, redis-scoped *(Q-pool-seam → §3.1; ADR-0230)*

**Decision:** The new capability — a terminal filter resolving a cluster host, dialing, pooling the TCP conn, and pipelining request↔reply in FIFO order — lands in the existing `internal/filter/network/` framework package as a genuine, reusable seam, but its surface is scoped to EXACTLY what redis needs now (catch_all single-cluster routing; one logical request stream per downstream connection; positional/FIFO reply correlation; basic per-host pooling). NO thrift-specific generalization (no method-routing abstraction, no pluggable-codec interface) is built speculatively. The seam REUSES the as-built `cluster.Manager` host-resolution + TCP dial path (the same one `tcp_proxy` uses) — it does not add a new connection primitive; it adds pooling + FIFO pipelining + the in-filter routing decision. Built on the existing `TerminalFilter.Handle(ctx, conn)` seam (redis_proxy is a terminal filter).

**Rationale:** Connection pooling to an upstream cluster is genuinely framework infrastructure, not redis-specific — thrift will need the same primitive (resolve host / dial / pool / round-trip). Placing it framework-level honors the seam-first split rationale (the 32.1 deliverable IS the seam) and avoids a disruptive promotion/refactor at the thrift phase. Scoping it to redis's needs (YAGNI) avoids the speculative-generalization trap: thrift's real requirements (method-routing, multiple protocols, a sub-filter chain) are not yet known, so a "designed-for-both" seam would over-fit guesses. The extract/generalize step happens at thrift's phase against thrift's actual needs (the project's extract-at-second-consumer discipline, as with BSON staying inside mongoproxy).

### 2.4 RESP reply correlation is POSITIONAL, not id-keyed *(self-answered — RESP has no correlation IDs)*

**Decision:** Unlike mongo (requestID↔responseTo) and kafka (`correlation_id`), the RESP protocol carries NO correlation identifier — replies return on a connection in the SAME ORDER as the requests were sent (pipelining). The seam therefore correlates replies to requests POSITIONALLY: per upstream connection, a FIFO pending-request queue; each reply dequeues the oldest pending request and is routed back to that request's originating downstream connection. The exact multiplexing model (one upstream conn per downstream conn, or a shared per-host pool with a per-conn pending queue) is pinned at SPEC (D32-5); the MVP may use the simplest faithful model (one upstream conn per downstream conn) and pin `upstream_cx_*` stats per-side where the reference's pooling internals diverge.

**Rationale:** RESP's in-order reply guarantee is the load-bearing invariant of any Redis proxy. Positional correlation is simpler than the id-keyed maps mongo/kafka needed (no per-connection correlation map, no mutex over an id→key map) but imposes a strict ordering contract on the pool (a reply MUST NOT be dequeued out of order). The MVP's single-route, single-key-command envelope keeps the pending queue trivially in-order (no fan-out/collate, which only multi-key fragmentation would introduce — deferred).

### 2.5 Differential strategy: synthesized RESP backend + cross-side StatsAsserter + downstream-response equivalence *(self-answered → fixture envelope §6)*

**Decision:** Hermetic fixtures with a NEW BackendKind — a synthesized **`TCPRedisResponder`** that speaks minimal RESP (canned replies for the exercised commands: `+PONG`, `+OK`, `$<n>\r\n<val>\r\n` bulk strings, `:<n>` integers, `-ERR …` errors); chain `[redis_proxy]` as the TERMINAL on BOTH sides (the contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` + envoy-go; NO `tcp_proxy` — redis_proxy terminates) routing catch_all to that backend; the driver sends real RESP commands as a Redis client would. The differential proof is TWO-pronged: (1) **downstream-response byte-equivalence** — the RESP bytes returned to the client must be byte-identical cross-side (the FIRST §9 row whose downstream RESPONSE bytes are the load-bearing proof, not just stats — because redis_proxy GENERATES the downstream response by proxying, it does not pass observed bytes through); (2) **cross-side `StatsAsserter`** over the mirrored `redis.<stat_prefix>.command.*` + downstream/upstream counters. `upstream_cx_*`/pooling stats that depend on connection-reuse internals are pinned per-side (not equality) where the reference diverges (`reference_close_direction_framework_gap` precedent). Every assertion proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). NO real Redis server.

**Rationale:** A terminal routing proxy's correctness is its downstream responses — so unlike the sniffer rows (where bytes pass through unchanged and the stat comparison IS the proof), the load-bearing proof here is downstream-response byte-equivalence PLUS stat parity. A synthesized `TCPRedisResponder` with canned replies keeps the upstream deterministic (a real Redis server adds container weight + nondeterministic RESP framing/version banners). The cross-side fixture boots on the contrib reference image (redis_proxy is a core extension, present in the standard image too — but the project standardized on the contrib image at ADR-0227, a behavioral superset; D32-1 reconfirms). The new BackendKind (anticipated value 32) is the redis analogue of the kafka correlation-id-echoing responder (BackendKind 31) — but FIFO/positional, not id-echoing (§2.4).

### 2.6 Deferred-active-feature posture: parse-accept vs parse-reject per field *(per Q1 — pinned at SPEC, D32-4)*

**Decision:** The deferred surface (§1.2 / §8) splits into (a) PARSE-ACCEPTED-behavior-DEFERRED fields (the config parses without error, kept at defaults in fixtures, behavior lands later — the mongo `header_delay` precedent) and (b) PARSE-REJECTED fields (config-load error — the strict posture for fields whose silent-ignore would be a correctness hazard). The per-field disposition is pinned empirically at SPEC (D32-4) by matching the reference Envoy's own parse behavior. Anticipated: `prefix_routes` beyond catch_all + `settings` sub-fields parse-accepted (they have safe defaults); `enable_redirection`/sharding + `faults` + mirroring + AUTH likely parse-accepted-deferred (defaults are off); the SPEC confirms which (if any) the reference rejects when set without their dependencies.

**Rationale:** Parse-accepting unconsumed fields keeps the config-parse surface faithful (the reference accepts a config with `settings` set; envoy-go must too) while the behavior lands later or stays a coverage boundary. The exact split is an empirical pin because silent-ignore-vs-reject is a per-field upstream-faithfulness question, not a design choice (`reference_wire_format_both_sides_see_same_bytes` generalized to config parse).

### 2.7 Stat surface: per-command + downstream + upstream counters under `redis.<stat_prefix>.` *(self-answered per §9 precedent; SPEC pins roster)*

**Decision:** Mirror upstream's redis stat families under the `redis.<stat_prefix>.` scope: per-command `command.<cmd>.*` counters (Redis has ~200+ commands → likely DYNAMIC per-command-seen creation with the `IsValidName` charset guard, NOT an eager roster like kafka's table-bounded 176 — pinned at SPEC, D32-2); downstream connection/request counters (`downstream_cx_*`, `downstream_rq_*`); and the upstream/pool counters (`upstream_cx_*`, `upstream_rq_*` — some of which may already exist as cluster stats). A new `redis.` Prometheus tag-extractor arm at `internal/stats/name.go` (the `.zookeeper.`/`.mongo.`/`kafka.` precedent; inline-vs-label-hoist pinned at SPEC, D32-2 — kafka's AMEND-K2 showed the arm may be the simplest INLINE shape). Wire-derived dynamic command segments guarded by `stats.IsValidName` before `NewCounterIfAbsent` (per `reference_dynamic_stat_name_charset_guard` — `NewCounterIfAbsent` PANICS on invalid names; the `FuzzRESPDecode` no-panic fuzzer needs the guard at the codec boundary). The command-latency HISTOGRAM family is DEFERRED project-wide (ADR-0060). Exact roster + eager-vs-dynamic creation = empirical pin at SPEC (D32-2). The stat-count DELTA is `536 → 536 + roster` (roster TBD at SPEC).

**Rationale:** Per-command counters are the load-bearing redis observability surface. Because the redis command space is large and open-ended (`custom_commands` exists, deferred), DYNAMIC per-command-seen creation is the likely shape (contrast kafka's closed 86-key table → eager) — the `IsValidName` guard is mandatory at the codec boundary (the charset-guard memory recurs for every protocol decoder). The exact roster is pinned at SPEC against the reference.

---

## 3. Framework-survey result — 1 NEW framework seam (the upstream connection-pool) + 1 NEW filter package + 0 new go.mod deps (anticipated)

### 3.1 NEW framework seam: the upstream connection-pool / cluster-routing seam *(per Q-pool-seam; ADR-0230; the SIXTH structural extension)*

Lands in the existing `internal/filter/network/` framework package (32.1). A terminal filter resolves a host of a named cluster via the as-built `cluster.Manager`, dials a TCP conn over the existing cluster TCP dial path (the same `tcp_proxy` uses), pools the conn per upstream host, and offers an in-order (FIFO) request→reply round-trip primitive (positional reply correlation — §2.4). Redis-scoped (Q-pool-seam): catch_all single-cluster routing, one logical request stream per downstream connection, basic per-host pooling — NO thrift-specific generalization. Anticipated near-minimal new exported surface (a small pool/round-trip type the redisproxy terminal calls); the exact API is pinned at the 32.1 SPEC (D32-3). Built on `TerminalFilter.Handle(ctx, conn)` (UNCHANGED).

### 3.2 NEW: `internal/filter/network/redisproxy/` *(per Q0+Q1; ADR-0229)*

Config parse (`stat_prefix` + `settings` minimum subset + `prefix_routes.catch_all_route.cluster`; the rest parse-accepted/rejected per D32-4) + the **RESP codec** (`resp.go` — request decode + reply decode + encode; value types + inline commands) + the `TerminalFilter.Handle` command→reply pump (read RESP request → route catch_all → seam round-trip → write RESP reply downstream) + the per-command/downstream/upstream counters. Implements `network.TerminalFilter` (NOT `ReadFilter`/`WriteFilter` — it terminates, it does not observe a chain).

### 3.3 go.mod deps: anticipated ZERO new *(self-answered; pinned at SPEC D32-1)*

redis_proxy is a CORE extension (`envoy.extensions.filters.network.redis_proxy.v3.RedisProxy`) — anticipated already present in the EXISTING `github.com/envoyproxy/go-control-plane/envoy v1.32.4` dep (contrast kafka_broker, which forced the FIRST `/contrib` dep). The RESP codec is in-house (byte scanning; no Redis client library). D32-1 confirms the module + import path + that `go mod tidy` adds nothing.

### 3.4 REUSES

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b/29.3) — the `TerminalFilter` seam (`Handle(ctx, conn)`), the registry, the builtins seam, the chainRuntime/Connection plumbing. The NEW upstream-pool seam (§3.1) lands IN this package.
- `internal/cluster/` (02/05.2) — the `cluster.Manager` host-resolution + TCP dial path (the same `tcp_proxy` uses); the upstream-pool seam builds ON it.
- `internal/filter/tcpproxy/` (02/26.2/27) — the existing terminal-filter + cluster-dial PRECEDENT (redis_proxy is a different terminal that ALSO dials a cluster — the dial path is shared, the pooling/pipelining is new).
- `internal/stats/` (06.1) — counters + `NewCounterIfAbsent` dynamic-name convention + `IsValidName` (the charset guard); the `internal/stats/name.go` Prometheus tag-extractor arm pattern (ADR-0138; `.rbac.`/`.zookeeper.`/`.mongo.`/`kafka.` precedents — the new `redis.` arm).
- The differential harness + `StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints) — booting the contrib reference image (ADR-0227).
- `envoy.extensions.filters.network.redis_proxy.v3` proto bindings (go-control-plane `/envoy` v1.32.4 — anticipated already a dep, §3.3).

---

## 4. Per-route applicability — none (network filters are not route-scoped)

Per the 26/29/31 BRAINSTORM §4 confirmation: network filters carry no `typed_per_filter_config` surface. redis_proxy's `prefix_routes` is its OWN internal routing table (not the HTTP route-config `typed_per_filter_config` mechanism), and only the `catch_all_route` leg is consumed at the MVP. Not applicable to phase 32.

---

## 5. Stat surface hypothesis

### 5.1 Per-command counters (SPEC pins)

`command.<cmd>.*` per Redis command (a DYNAMIC family keyed on the command name, created per-command-seen with the `IsValidName` guard — D32-2; contrast kafka's eager table-bounded roster). The exact sub-counters (`.total`/`.success`/`.error`/…) + whether any are eager + the latency-histogram deferral are pinned at SPEC.

### 5.2 Downstream + upstream/pool counters (SPEC pins)

Downstream `downstream_cx_*` / `downstream_rq_*` + upstream/pool `upstream_cx_*` / `upstream_rq_*` (some may already exist as cluster stats). The exact roster + which upstream/pool counters are differentially-mirrored vs pinned per-side (pooling internals diverge — §2.5) is pinned at SPEC.

### 5.3 Project stat count delta

536 → **536 + roster** (the roster = the exercised command set across `command.*` + the fixed downstream/upstream counters; the EXACT roster + eager-vs-dynamic creation pinned at SPEC, D32-2). Any upstream counters NOT mirrored land as BEHAVIOR_CONTRACT coverage-boundary records.

### 5.4 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The command-latency HISTOGRAM family unmirrored (ADR-0060); the deferred-active-feature stats (sharding/redirection/fragmentation/AUTH/faults/mirroring counters — parse-accepted-or-rejected fields, behavior + stats deferred); pooling/`upstream_cx_*` per-side asymmetry (§2.5); any upstream runtime-key gating (envoy-go has no runtime layer → key defaults; the Runtime family row stays the future home).

---

## 6. Differential fixture envelope — anticipated two directories

### 6.1 Fixtures (+2)

- **`0055-redis-roundtrip`** (cross-side): chain `[redis_proxy]` as the terminal on BOTH sides, catch_all → the new `TCPRedisResponder` backend; the driver sends real RESP commands (PING/GET/SET + a command spread + an error/edge arm at 32.2) as a Redis client; the proof is downstream-RESP-response byte-equivalence (§2.5 prong 1) PLUS the cross-side `StatsAsserter` over `command.*`/downstream/upstream counters (prong 2); one deliberate-break liveness proof per arm with `-count=1`. (The 32.1 round-trip arm + the 32.2 command-matrix arms share this one cross-side dir — the cross-side-XOR-boot-reject fixture-dispatch constraint allows multiple cross-side arms in one dir, `reference_differential_fixture_dispatch_constraint`.)
- **`0056-redis-boot-reject`** (boot-reject; separate dir per the cross-side-XOR-boot-reject constraint): a required-field / invalid-config arm that both sides reject at boot (the exact arm — e.g. missing `stat_prefix`, or a catch_all referencing an undefined cluster — pinned at SPEC, D32-4). (Include-or-fold pinned at SPEC.)

### 6.2 Total

56 → **58** (with both dirs; 57 if the boot-reject arm folds). SPEC pins exact numbering + arm rosters. The 32.1 IMPL lands `0055`'s round-trip arm (+ `0056` if pinned there); the 32.2 IMPL extends `0055` with the command matrix.

### 6.3 New BackendKind

A NEW BackendKind (anticipated value **32**) — a synthesized `TCPRedisResponder` speaking minimal RESP (canned replies for the exercised commands: `+PONG`/`+OK`/bulk-string/integer/error). Contrast the existing silent `TCPSink` (28) / the `TCPMongoResponder` (30) / the `TCPKafkaResponder` (31 — correlation-id-echoing). The redis responder is FIFO/positional (no correlation id — §2.4); the exact canned-reply table is pinned at SPEC.

### 6.4 No conformance harness

No new conformance harness (matches 26/27/28/29/31). The h2spec + proxy-wasm gates re-run asserted-unaffected at the six-gate (image-independent; phase 32 touches no HTTP/h2/proxy-wasm path).

---

## 7. Anticipated ADRs — 2 ADRs (ADR-0229 + ADR-0230)

Next-free ADR at master tip is **ADR-0229** (DECISIONS.md tail ADR-0228 — the phase-31 kafka_broker filter; the ADR-0209 escape-valve reserve stands unconsumed).

- **ADR-0229** *(32 — filter + parent umbrella)* — the `redis_proxy` filter: the single-route-terminal envelope (RESP codec + catch_all single-cluster routing + the core single-key command set + per-command/downstream/upstream stats), the `redis.` prom tag-extractor arm, the 10th built-in, the deferred-active-feature posture (parse-accept-vs-reject per field; sharding/redirection/fragmentation/AUTH/faults/mirroring deferred), and the cross-side differential (downstream-response byte-equivalence + stat parity). The parent-row umbrella ADR for the 2-way split.
- **ADR-0230** *(32.1 — the upstream connection-pool / cluster-routing seam)* — the framework's SIXTH structural extension: a terminal filter resolves a cluster host, dials, pools, and pipelines request↔reply in FIFO order; redis-scoped (no thrift generalization); builds on `TerminalFilter.Handle` + the as-built `cluster.Manager` dial path. The seam ADR (the analogue of ADR-0221 [WriteFilter seam] / ADR-0226 [async halt/resume seam]).

§Context drafts land at the parent SPEC (ADR-0229) and the 32.1 SPEC (ADR-0230); §Decision/§Consequences bodies at the respective IMPL per ADR-0044. Anticipated default is 2 ADRs (next-free after phase 32 ≈ **ADR-0231**); the per-sub-phase SPEC/PLAN may surface additional ADRs (each re-checks).

---

## 8. Deferred items

- **`prefix_routes` multi-cluster routing beyond `catch_all`** — route-by-key-prefix to multiple clusters (`remove_prefix`, `case_insensitive`, per-route mirror policy). The catch_all leg is the MVP; the prefix table extends the upstream-pool seam's routing decision. Parse-accepted-behavior-deferred (D32-4).
- **Hash-ring sharding + `enable_redirection` (MOVED/ASK)** — Redis Cluster-mode host selection by key hash + redirection handling. A future routing sub-phase (extends the seam's host-selection).
- **Multi-key command fragmentation (MGET/MSET/DEL split-and-collate)** — splitting a multi-key command across shards + collating the replies. Introduces fan-out/collate (breaks the trivial FIFO pending queue — §2.4); a future sub-phase.
- **Downstream AUTH** (`downstream_auth_password`/`downstream_auth_username`/`external_auth_provider`) — a future auth sub-phase.
- **`faults`** (error/delay fault injection) — a future sub-phase (the redis analogue of the mongo fault-delay; may consume the ADR-0226 halt machinery or a redis-local timer).
- **Request mirroring** (`request_mirror_policy`) — a future sub-phase.
- **Replica `read_policy`** (route reads to replicas) — a future sub-phase (extends host selection).
- **`custom_commands`** — a future sub-phase.
- **The full `ConnPoolSettings` surface** (op timeouts, flush batching, `max_upstream_unknown_connections`, `connection_rate_limit`, `buffer_flush_timeout`) — the MVP consumes a minimum subset; the rest parse-accepted-deferred.
- **Command-latency histograms** — deferred per ADR-0060; coverage-boundary record.
- **Runtime-key gating** — no runtime layer exists; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot restart family row is the future home).
- **Real-Redis-server integration fixtures** — out of scope; the hermetic synthesized `TCPRedisResponder` only.
- **The remaining protocol proxy** — `thrift` — its own future family phase, needing its own brainstorm-time risk assessment (transport×protocol matrix + method-routing + nested thrift-filter sub-chain). Thrift REUSES the 32.1 upstream-pool seam.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **31 BRAINSTORM §8 / 29 BRAINSTORM §8 (the remaining protocol proxies)** — `redis` PICKED UP here; `thrift` carried (the last §9 candidate; reuses the 32.1 upstream-pool seam).
- **31 BRAINSTORM §1.3 (the terminal-proxy risk assessment owed by redis/thrift)** — DISCHARGED here: redis_proxy needs the upstream connection-pool / cluster-routing seam (a framework first), scoped framework-level + redis-only, landed seam-first at 32.1; the MVP is the single-route terminal (no observational MVP exists for a terminal proxy).
- **ADR-0221 §Consequences / ADR-0226 (the WriteFilter + async halt/resume seam consumers)** — NOT consumed by phase 32 (a terminal routing proxy owns both connection ends directly; it does not observe a `tcp_proxy`-terminated chain). The upstream-pool seam (ADR-0230) is the terminal-proxy analogue.
- **`tcp_proxy` `downstream_cx_*` family unmirrored** (phase-27 record) — redis_proxy is itself a terminal with its OWN `downstream_cx_*` surface (D32-2); whether it closes the phase-27 carry is pinned at SPEC.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

The parent-SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25/26/27/28/29/31 SPEC precedent) against the contrib reference image (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) + go-control-plane `/envoy` v1.32.4 bindings, using the live-probe precedent (`reference_docker_probe_bridge_network`):

- **D32-1** *(SPEC-BLOCKING for the dep + TypeURL)* — verify the TypeURL via `proto.MessageName` on the `RedisProxy` proto (the `extensions.` lesson, `reference_network_filter_typeurl_extensions` — confirm `envoy.extensions.filters.network.redis_proxy.v3.RedisProxy`, NOT the docs string); confirm the module + import path (anticipated `github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3` — CORE, NOT `/contrib`) AND that `go mod tidy` adds ZERO new dep with the first consumer.
- **D32-2** *(SPEC-BLOCKING for the stat roster)* — live-probe the contrib reference Envoy (bridge-network probe per `reference_docker_probe_bridge_network`; STRICT_DNS backend hostname; verify the round-trip actually ran via `downstream_cx_rx_bytes_total > 0` AND a non-empty downstream RESP response) for the EXACT stat roster: the `command.<cmd>.*` naming + sub-counters, eager-vs-dynamic creation, the downstream/upstream counter set, which upstream/pool counters are differentially-mirrorable vs per-side-pinned, and the `redis.` Prometheus tag-extractor form (inline-vs-label-hoist — the kafka AMEND-K2 lesson that the arm may be the simplest INLINE shape). Also pin the `settings.enable_command_stats` field's default + effect (it directly governs whether the per-command `command.<cmd>.*` counters are emitted) — the MVP fixtures must set it to match the per-command-counter expectation.
- **D32-3** *(SPEC-BLOCKING for the seam API)* — pin the upstream connection-pool / cluster-routing seam's exact surface (the new exported type(s) the redisproxy terminal calls; the host-resolution + dial reuse of `cluster.Manager`; the FIFO pending-queue / positional-correlation contract; the multiplexing model — one upstream conn per downstream conn vs shared per-host pool); confirm it builds on `TerminalFilter.Handle` + the existing TCP dial path with near-minimal new API.
- **D32-4** — the config PGV / parse arms: which fields are PARSE-REJECTED (config-load error) vs PARSE-ACCEPTED-behavior-DEFERRED, by matching the reference Envoy's own parse behavior (`stat_prefix` required? catch_all-references-undefined-cluster rejected? `settings`/`prefix_routes`/sharding/faults/AUTH set-without-deps accepted or rejected?); + the `0056-redis-boot-reject` arm selection + include-or-fold.
- **D32-5** — the RESP wire format adopted VERBATIM (`reference_wire_format_both_sides_see_same_bytes`): the value-type framing (`+`/`-`/`:`/`$`/`*` + CRLF + inline commands), the request array shape, the reply shapes for the exercised commands; enough to hand-craft the `TCPRedisResponder` canned replies + the driver requests + the downstream-response byte-equivalence assertion. + the positional reply-correlation contract (RESP has no correlation id — §2.4) + the `upstream_cx_*` per-side pooling-stat asymmetry (`reference_close_direction_framework_gap` precedent).
- **D32-6** *(SPEC-BLOCKING for the round-trip + fixture traffic)* — PING/AUTH local-reply semantics: does the reference Envoy answer PING (and AUTH, when no downstream auth is configured) LOCALLY without dialing upstream, or proxy it to the backend? This is BLOCKING because whether PING hits the backend changes BOTH the `TCPRedisResponder` expected traffic AND the round-trip stat counts (and therefore the `0055` fixture's exercised commands + the downstream-response byte-equivalence arm). Pin it before authoring the fixture.
- **D32-7** — the `stats.IsValidName` charset-guard placement at the codec boundary (guard the wire-derived dynamic command segment before `NewCounterIfAbsent`; the `FuzzRESPDecode` no-panic fuzzer crashes without it — `reference_dynamic_stat_name_charset_guard`).
- **D32-8** — the ADR-0045 task/LoC envelope per sub-phase confirming the 2-way split holds (32.1 = seam + codec + round-trip; 32.2 = commands + stats + matrix; each re-checked at its PLAN); whether a 32.x escape-valve split is needed; + whether redis_proxy needs any close-direction-keyed counters (the `reference_close_direction_framework_gap` lesson — if so, defer to a framework-surgery boundary).

---

## 11. Prior-phase lessons applied

- **TypeURL via `proto.MessageName`, never the docs string** (`reference_network_filter_typeurl_extensions`; the `extensions.` carrier). Applied: pinning-test at the 32.1 IMPL Task 1 on the `RedisProxy` proto (D32-1).
- **Wire-format pins: both sides see the same bytes** (`reference_wire_format_both_sides_see_same_bytes`; the 28.2 watch-event / 31 kafka lessons). Applied: ALL RESP framing/reply shapes are empirical pins against the contrib reference Envoy (D32-5); "our frame format" is never a valid deviation. For redis the principle EXTENDS to the downstream RESPONSE bytes (the proxy generates them — §2.5).
- **The reference parses the full message** (`reference_differential_reference_parses_full_message`; cross-side fixtures need fully-valid frames + the reference may add an abandon-at-close failure; pin per-side `*.failure`/`*_cx_*` values, not equality). Applied: the `TCPRedisResponder` replies are fully-valid RESP; pooling/`upstream_cx_*` divergences pinned per-side (D32-5).
- **Dynamic stat-name charset guard** (`reference_dynamic_stat_name_charset_guard`; `NewCounterIfAbsent` PANICS on invalid names). Applied: the wire-derived command segment is guarded by `stats.IsValidName` at the codec boundary (D32-7); the `FuzzRESPDecode` no-panic fuzzer needs it. (Redis commands are likely DYNAMIC, unlike kafka's table-bounded eager roster — so the guard is load-bearing, not satisfied-by-construction.)
- **Close-direction is a framework gap** (`reference_close_direction_framework_gap`; the framework records close TYPE not DIRECTION). Applied: confirm at the probe whether any redis counter is close-direction-keyed (D32-8); if so, defer to a framework-surgery boundary rather than touch the framework here.
- **Docker probes need a bridge network** (`reference_docker_probe_bridge_network`; Docker Desktop netns). Applied: the D32-2/D32-5 live probes use a shared bridge network + STRICT_DNS backend hostname + verify the round-trip ran via `downstream_cx_rx_bytes_total > 0` + a non-empty downstream RESP response.
- **Differential break protocol needs `-count=1`** (`reference_differential_break_protocol_count1`; go-test result caching serves a stale PASS). Applied: every R4 deliberate-break in phase 32 runs with `-count=1`.
- **Differential asserter dispatch + liveness** (`reference_differential_asserter_dispatch`; cross-side fixtures use `StatsAsserter`, prove every assertion live). Applied: the cross-side `StatsAsserter` + the downstream-response byte-equivalence are the load-bearing proofs; every fixture arm records a deliberate-break.
- **Cross-side XOR boot-reject per fixture dir** (`reference_differential_fixture_dispatch_constraint`). Applied: `0055` cross-side (multi-arm); `0056` boot-reject; never mixed.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`). Applied at every IMPL.

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 32 = `redis_proxy`, the SIXTH §9 Network-filters row and the project's FIRST terminal routing proxy (chosen over thrift — RESP is far leaner than Thrift's transport×protocol matrix + method-routing + sub-filter chain); (Q1) a SINGLE-ROUTE-TERMINAL MVP — RESP codec + `catch_all_route` to one upstream cluster + a basic per-host upstream connection pool with FIFO/positional request↔reply correlation + the core single-key command set + per-command/downstream/upstream stats; sharding/redirection/fragmentation/AUTH/faults/mirroring/read_policy/custom_commands deferred (parse-accept-vs-reject per field pinned at SPEC); histograms deferred (ADR-0060); (Q-split) a 2-way FEATURE-PROGRESSIVE pre-split — parent row 32 + 32.1 (the upstream connection-pool / cluster-routing FRAMEWORK SEAM + the RESP codec + a minimal catch_all round-trip, seam-first) + 32.2 (the full command set + the stat roster + the differential command matrix + the fuzzer); (Q-pool-seam) the new seam lands framework-level in `internal/filter/network/` but redis-scoped (YAGNI; thrift reuses/extends it later). Self-answered per §9 precedent: positional (not id-keyed) reply correlation (RESP carries no correlation id); the differential proof is downstream-RESP-response byte-equivalence PLUS cross-side `StatsAsserter` (the FIRST §9 row whose downstream response bytes are load-bearing — redis_proxy generates them); the `redis.` prom tag-extractor arm + the `IsValidName` charset guard; anticipated ZERO new go.mod dep (redis_proxy is core `/envoy`, not `/contrib`). ONE new framework seam (the upstream connection-pool — the SIXTH structural extension) + one NEW filter package (`redisproxy`) + zero new go.mod deps (anticipated). Anticipated 2 ADRs (ADR-0229 filter/parent + ADR-0230 seam), fixtures 56 → 57–58 (`0055-redis-roundtrip` cross-side + `0056-redis-boot-reject`), stat surface 536 → 536 + roster (TBD at SPEC), fuzzers 40 → 41 (`FuzzRESPDecode`), BackendKind 31 → 32 (the `TCPRedisResponder`). After phase 32 the §9 candidate {thrift} remains.

The next session authors `docs/envoy-go/phases/32-network-filter-redis-proxy/SPEC.md` (`superpowers:writing-plans` scoped to parent SPEC authoring), executing the §10 D32-1..D32-8 empirical pins IN-SESSION against the contrib reference Envoy per ADR-0004/ADR-0227, formalizing the 2-way split surface-mapping, and anchoring the ADR-0229 §Context draft (ADR-0230 §Context lands at the 32.1 SPEC). Per ADR-0106(d), parent row 32 registers `in-progress` with sub-phases `32.1, 32.2` at this BRAINSTORM-DONE commit.
