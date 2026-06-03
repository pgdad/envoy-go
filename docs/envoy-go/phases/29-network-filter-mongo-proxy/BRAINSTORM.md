# Phase 29 Brainstorm — `mongo_proxy` + the async halt/resume seam (parent row; FOURTH §9 Network-filters-family row)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 29 (`network-filter-mongo-proxy`), the **FOURTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, and the phase-28 `zookeeper_proxy` parent row). Phase 29 lands `envoy.filters.network.mongo_proxy` — a passive observability sniffer that decodes the MongoDB legacy wire protocol in **both directions** and emits per-opcode / per-command / per-collection counters plus a gauge — together with **fault-delay injection** (the `delay` proto field), whose halt/resume requirement drives the phase's framework-seam extension.

The next session (lifecycle-state 1 → 2 for phase 29, skill `superpowers:writing-plans` scoped to **parent SPEC authoring** per the phase 22 / 24 / 25 / 26 / 28 parent-row precedent) authors `docs/envoy-go/phases/29-network-filter-mongo-proxy/SPEC.md` based on this brainstorm — that parent SPEC formalizes the 3-way split surface-mapping + executes the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004. The per-sub-phase SPEC sessions (29.1 / 29.2 / 29.3) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the 22.1 / 25.1 / 26.1 / 28.1 precedent.

**Brainstorm session:** worktree `.worktrees/phase-29-network-filter-mongo-proxy-brainstorm`, branch `phase-29-network-filter-mongo-proxy-brainstorm`, branched from master tip `0a2cf8e` (`next-prompt.txt: repoint master-tip reference to fd709a6` — a docs-only repoint commit). Substantive predecessor on master: `fde21c9` (the phase-28.2 IMPL squash — zookeeper_proxy response decoder + per-connection mutex + latency counters + the phase-28 ATOMIC rollup, ADR-0223).

**Brainstorm mode:** interactive with a live human. The user picked the subject + each major design decision via a 4-question dialogue:

- **Q0 subject selection** — `mongo_proxy` chosen from the 4 remaining §9 Network-filters candidates {redis / mongo / kafka_broker / thrift}, plus the option of any other §9 family. Rationale: the natural next per the ADR-0221 §Consequences anticipation (consumer #2 of the `network.WriteFilter` conn-wrap seam); a both-direction sniffer with per-opcode stats directly analogous to zookeeper_proxy; a core extension (no contrib-image risk); the smallest delta from the as-built infrastructure. (`redis`/`thrift` are large terminal-proxy surfaces needing their own routing/pooling seams; `kafka_broker` is an Envoy **contrib** extension with reference-image availability risk.)
- **Q1 scope envelope** — `Full parity minus histograms` chosen from {Full parity minus histograms (3-way pre-split) / Counters only, defer fault-delay + access log (2-way pre-split) / Op-code counters only (minimal MVP)}. The filter's complete COUNTER + GAUGE surface is mirrored (both-direction op-code counters incl. the `op_query_active` gauge, per-command `cmd.<cmd>.*` and per-collection `collection.<collection>.query.*` counters via in-house BSON decode), PLUS fault-delay injection, the mongo access log, and dynamic metadata; the `reply_num_docs`/`reply_size`/`reply_time_ms` HISTOGRAM families stay deferred per ADR-0060 (recorded as a coverage boundary).
- **Q2 fault-delay seam shape** — `Upstream-faithful halt/resume (StopIteration + ContinueReading)` chosen from {ContinueReading resume seam / Go-idiomatic blocking delay / Defer fault-delay}. Upstream injects delay by returning `StopIteration` from `onData` while a delay timer pends, then calling `read_callbacks_->continueReading()` when the timer fires. **As-built correction discovered during this brainstorm:** `ReadFilterCallbacks.ContinueReading()` ALREADY EXISTS (landed at 26.1 — `internal/filter/network/callbacks.go` + `chain.go`), but it supports only (a) resuming an OnNewConnection halt and (b) re-entrant in-OnData resume; it explicitly does NOT support a persistent OnData halt with later asynchronous resume, and post-handoff the `readChainConn` replay path IGNORES filter Status entirely (the 28.1b SPEC §3.5 observational boundary). The seam phase 29 lands is therefore an **extension of the existing machinery**: OnData-halt persistence + cross-goroutine-safe `ContinueReading` + post-handoff read-halt honoring. See §2.3.
- **Q3 differential strategy** — `Synthesized-bytes + StatsAsserter` chosen from {Synthesized-bytes hermetic fixtures + cross-side StatsAsserter / Real mongod in the loop / Unit tests + passthrough-only fixture}. Hand-crafted MongoDB wire-protocol bytes (little-endian MsgHeader + BSON documents) driven through a `[mongo_proxy, tcp_proxy]` chain on BOTH sides; the mirrored counters compared cross-side via `StatsAsserter`; deliberate-break liveness proof per fixture; fault-delay arms made deterministic via 100%-probability delay + the `delay_injected` counter (timing itself never compared).

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0223), and the as-built §9 framework (26.1/26.2/26.3/27/28). Empirical pins requiring evidence against Envoy v1.37.2 are enumerated in §10 and deferred to parent-SPEC-drafting time per the phase 09–28 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/28-network-filter-zookeeper-proxy/BRAINSTORM.md` section-for-section (the most recent parent pre-split precedent), reframed for the mongo_proxy scope + the 3-way pre-split. Phase 29 sits in a structurally meaningful position: it is **consumer #2 of the ADR-0221 `network.WriteFilter` seam** (exactly as that ADR's §Consequences anticipated); it **extends the 26.1 `ContinueReading` halt/resume machinery into a full async-resume seam** (lifting, for halt purposes, the 28.1b §3.5 post-handoff observational boundary); and it is the family's **second stats-PRIMARY filter** with the project's first GAUGE-bearing differential surface. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-02.

---

## 1. Mission and scope confirmation (29 only)

ROADMAP row `29 | network-filter-mongo-proxy | 28 | in-progress | 29.1, 29.2, 29.3 | …` (added by this brainstorm) is the parent row this brainstorm registers as `in-progress` with sub-phase list `29.1, 29.2, 29.3`. The three sub-rows `29.1 | network-filter-mongo-wire-and-requests | 28 | planned | | …`, `29.2 | network-filter-mongo-responses-and-correlation | 29.1 | planned | | …`, and `29.3 | network-filter-mongo-fault-delay-and-access-log | 29.2 | planned | | …` are also registered by this brainstorm (long-prefix slug convention; phase-22/25/26/28 precedent). The phase-28.2 IMPL squash `fde21c9` is the parent row's `depends-on` anchor.

The Network filters family candidate roster at `ROADMAP.md` (§ Feature Families) immediately BEFORE this brainstorm's registration commit was: `redis, mongo, kafka_broker, thrift` (echo, direct_response, sni_cluster, rbac_network, zookeeper — DONE via phases 26/27/28). Phase 29 lands **`mongo`** (this commit updates the heading to mark mongo IN-PROGRESS). After phase 29 phase-done, **3** family candidates remain (`redis`, `kafka_broker`, `thrift`). Branch/directory/Go-package identifiers: parent branch `phase-29-network-filter-mongo-proxy-brainstorm`, parent directory `29-network-filter-mongo-proxy/`, filter package `internal/filter/network/mongoproxy/` (Go package `mongoproxy`, single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy` precedent).

Phase 29 is also: (i) **consumer #2 of the ADR-0221 `network.WriteFilter` seam** — the filter implements BOTH `ReadFilter` and `WriteFilter` (one instance, both directions), receiving upstream→downstream bytes via `OnWrite` exactly as zookeeper_proxy does; per project memory `reference_network_chain_terminal_handoff_ends_ondata`, implementing `WriteFilter` (even where a method is a no-op) is what qualifies the chain for the 28.1b read seam. (ii) the FIRST consumer of **persistent OnData halt + asynchronous `ContinueReading`** — upstream mongo_proxy's fault-delay returns `StopIteration` from `onData` while a delay timer pends and resumes via `continueReading()` from the timer callback; the as-built 26.1 machinery must be extended for this (§2.3). (iii) the family's **second stats-PRIMARY filter** and the project's **first gauge-bearing differential surface** (`op_query_active`) — the cross-side `StatsAsserter` comparison is again the load-bearing proof. (iv) the FIRST filter requiring **in-house BSON decode** (little-endian binary JSON) — MongoDB framing is little-endian throughout, in contrast to zookeeper's big-endian jute.

### 1.1 What phase 29 delivers as a self-contained whole (envelope: full parity minus histograms per Q1)

Phase 29 lands `envoy.filters.network.mongo_proxy` at full counter+gauge parity + fault-delay + access log, across THREE sub-phases:

1. **Sub-phase 29.1** (`network-filter-mongo-wire-and-requests`) — delivers: (a) the **`internal/filter/network/mongoproxy/` package request side** — TypeURL via `proto.MessageName` (the `extensions.` lesson, `reference_network_filter_typeurl_extensions`), config parse of the 5-field `MongoProxy` proto (`stat_prefix` PGV-required → boot-reject; `access_log` path; `delay` FaultDelay; `emit_dynamic_metadata`; `commands` list — parse all five at 29.1, consume `delay`/`access_log`/`emit_dynamic_metadata` at 29.2/29.3), the **in-house BSON parser** (little-endian; `encoding/binary` only; document/element walk sufficient for command-name + collection + query-flag extraction), the **MongoDB legacy wire decoder request side** (little-endian MsgHeader `messageLength`/`requestID`/`responseTo`/`opCode` + OP_QUERY / OP_INSERT / OP_GET_MORE / OP_KILL_CURSORS structures + decoder-internal partial-packet reassembly + unconditional passthrough), request-side counters (`op_query`, `op_insert`, `op_get_more`, `op_kill_cursors`, the `op_query_*` flag counters, `op_query_scatter_get`/`op_query_multi_get`, `decoding_error`), per-command `cmd.<cmd>.total` + per-collection `collection.<collection>.query.*` dynamic counters, and the **per-connection active-query list** laid down for 29.2 correlation; (b) registration as the **8th built-in** (`builtins.RegisterBuiltins` single insertion) + the `mongo_proxy/v3` proto blank-import in `internal/bootstrap/bootstrap.go`; (c) the **`.mongo.` Prometheus inline-prefix arm** at `internal/stats/name.go` (the ADR-0138 / `.zookeeper.` precedent); (d) fixtures **`0049-mongo-requests`** (cross-side) + **`0050-mongo-boot-reject`**; (e) the **39th fuzzer** (wire+BSON request decode); (f) the BEHAVIOR_CONTRACT 29.1 bundle + STATE/ROADMAP advance.

2. **Sub-phase 29.2** (`network-filter-mongo-responses-and-correlation`) — delivers: (a) the **response-side decoder** in `OnWrite` — OP_REPLY decode (responseFlags + cursorID + numberReturned) + reply-flag counters (`op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`); (b) **requestID↔responseTo correlation** consuming the 29.1 active-query list + the **`op_query_active` GAUGE** (inc at query decode, dec at correlated reply — the project's first differentially-mirrored gauge) + `cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq`; (c) **dynamic-metadata emission** (`emit_dynamic_metadata` → the connection-scoped ADR-0217 `*dynamicmetadata.Bucket`; namespace/keys pinned at SPEC); (d) fixture **`0051-mongo-responses`** (cross-side); (e) the BEHAVIOR_CONTRACT 29.2 bundle (+ the 40th fuzzer if the SPEC pins a separate response-decode fuzzer).

3. **Sub-phase 29.3** (`network-filter-mongo-fault-delay-and-access-log`) — delivers: (a) the **async halt/resume seam extension** (§2.3; ADR-0226) — OnData-halt persistence, cross-goroutine-safe `ContinueReading`, post-handoff read-halt honoring in `readChainConn`; (b) **fault-delay injection** (the `delay` FaultDelay field: percentage + fixed delay; `delay_injected` counter; deterministic differential arms via 100%-probability); (c) the **mongo access log** (the `access_log` proto field; reuses `internal/accesslog`'s file-sink/async-writer machinery; format + field mapping pinned at SPEC); (d) `cx_drain_close` (posture vs the phase-08.2 drain machinery pinned at SPEC); (e) fixture **`0052-mongo-fault-delay`** (cross-side); (f) the BEHAVIOR_CONTRACT 29.3 bundle + the parent-row-29 ROLLUP (parent flips `in-progress → done` ATOMICALLY with sub-row 29.3 per the 18/19/22/24/25/26/28 precedent) + the six-gate.

### 1.2 What phase 29 does NOT deliver (forward to §8)

See §8. Highlights: the `reply_*` histogram families (ADR-0060); OP_MSG / modern-protocol decode IF upstream v1.37.2 does not decode it (empirical pin D29-4 — mirrored only to upstream's actual envelope); real-MongoDB-server fixtures; runtime-key gating (`mongo.proxy_enabled` etc. — no runtime layer exists; envoy-go-strict departure); the remaining protocol proxies (`redis`/`kafka_broker`/`thrift`).

### 1.3 Phase-done as the FOURTH Network-filters-family row landing

After phase 29, the family candidate count drops 4 → **3** (`redis`, `kafka_broker`, `thrift`). Both anticipated consumers of the ADR-0221 WriteFilter seam (zookeeper_proxy, mongo_proxy) will have landed; the remaining candidates are terminal-proxy-shaped (`redis`/`thrift`) or contrib-risky (`kafka_broker`) — each needs its own brainstorm-time risk assessment.

### 1.4 ADR-0045 split readiness — 3-way pre-split chosen per Q1

Per ADR-0045 §6, the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 29's full surface clearly exceeds the gate as a single phase:

- The BSON parser (document/element walk, command/collection/flag extraction): ~250–400 LoC.
- The wire decoder, request side (framing + 4 request opcodes + counters + active-query list): ~350–500 LoC.
- The response side (OP_REPLY + correlation + gauge + metadata): ~250–400 LoC.
- The async halt/resume seam + fault delay + access log: ~400–600 LoC.
- Fixtures (4 dirs incl. hand-crafted mongo byte corpora) + fuzzer(s) + prom arm + docs: ~400–600 LoC.
- Task counts: ~14–18 (29.1) + ~10–14 (29.2) + ~12–16 (29.3) ≈ 36–48 total — far over the gate as one phase; comfortably under it per sub-phase.

Total anticipated ~1650–2500 LoC / ~36–48 tasks → the 3-way pre-split at BRAINSTORM time (the project's FIFTH BRAINSTORM-time pre-split after 22/25/26/28). The split axis is FEATURE-PROGRESSIVE and consume-at-consumer-ordered: 29.1 = decode foundation + request side (independently shippable: a request-side-observable mongo filter with live stats + fixtures); 29.2 = response side + correlation (completes the both-direction counter surface); 29.3 = the seam + fault delay + access log (the framework extension lands WITH its first consumer, never speculatively). Each sub-phase re-checks the gate at its own PLAN time.

### 1.5 Seed-stub alignment + package naming

No seed-stub for mongo exists. Phase 29.1 creates `internal/filter/network/mongoproxy/` from scratch (package `mongoproxy`; matches the `directresponse`/`snicluster`/`zookeeperproxy` single-token-joined convention). The BSON parser lives INSIDE the mongoproxy package (`bson.go`) — NOT a new top-level `internal/bson/` package (YAGNI; extract-at-second-consumer if a future filter needs BSON). The halt/resume seam extension lands IN the existing `internal/filter/network/` framework package (chain.go / readconn.go / callbacks.go) — NOT a new package.

### 1.6 No prebrainstorm-notes branch

No `phase-29-*-prebrainstorm-notes` branch exists. Phase 29 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 29's relationship to prior framework deltas

Phase 29 continues the framework-delta-GROWTH posture. Prior lineage (abridged; see 26 BRAINSTORM §1.7 + 28 BRAINSTORM §1.7): 07.1 HTTP filter framework → … → 26.1 `internal/filter/network/` read-filter framework (incl. the original `ContinueReading()` + halt machinery) → 26.2 `TerminalFilter` seam → 26.3 `internal/rbac/` + connection-scoped dynamic-metadata → 27 the connection-scoped upstream-cluster-override seam (ADR-0219) → 28.1a the `network.WriteFilter` seam (ADR-0221) → 28.1b the post-handoff read seam (`readChainConn`/`replayRead`). **Phase 29.3 — the async halt/resume seam extension** (the framework's FIFTH structural extension): OnData-halt persistence + cross-goroutine `ContinueReading` + post-handoff read-halt honoring. This extends machinery that already exists (the 26.1 halt/resume + the 28.1b read seam) rather than adding a new interface — the seam's "newness" is in its semantics (persistence, async resume, post-handoff effectiveness), not its API surface (anticipated: zero or near-zero new exported methods; SPEC pins).

---

## 2. Design decisions

### 2.1 Subject selection: `mongo_proxy` *(Q0 → phase 29 row registered)*

**Decision:** Phase 29 = `envoy.filters.network.mongo_proxy` (proto `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`; bindings present in go-control-plane v1.32.4 — verified locally at brainstorm time: `stat_prefix`/`access_log`/`delay`/`emit_dynamic_metadata`/`commands`).

**Rationale:** The natural next per the ADR-0221 §Consequences anticipation (consumer #2 of the WriteFilter conn-wrap seam). A both-direction passive sniffer with per-opcode stats — the zookeeper_proxy pattern transfers nearly section-for-section. A core upstream extension at `source/extensions/filters/network/mongo_proxy` (NOT contrib — no reference-image risk; D29-1 confirms by boot). `redis`/`thrift` are large terminal-proxy surfaces (command routing + upstream pooling — they need framework seams of their own); `kafka_broker` carries contrib-image risk that could kill the cross-side differential strategy.

**Anticipated ADRs:** ADR-0224 (the mongo_proxy filter + request side; see §7).

### 2.2 Scope envelope: full counter+gauge parity; histograms deferred *(Q1 → 3-way pre-split; ADR-0224/0225/0226)*

**Decision:** Mirror upstream's complete per-opcode + per-command (`cmd.<cmd>.total`) + per-collection (`collection.<collection>.query.total`/`scatter_get`/`multi_get`) COUNTER surface in both directions, PLUS the `op_query_active` GAUGE, PLUS fault-delay injection (`delay_injected`), PLUS the mongo access log, PLUS dynamic metadata. The `reply_num_docs`/`reply_size`/`reply_time_ms` HISTOGRAM families (per-command and per-collection) are NOT mirrored — deferred per ADR-0060 (the project-wide histogram deferral), recorded as a BEHAVIOR_CONTRACT coverage boundary (the `*_response_latency` precedent from phase 28).

**Rationale:** mongo_proxy is a stats-PRIMARY filter — a partial stat surface would gut its purpose. The gauge is mirrorable with existing machinery (`internal/stats/gauge.go` landed at 06.1). The BSON decode the cmd/collection counters need is bounded (document/element walk; no third-party dep). Fault delay is the feature the framework's halt-semantics deferral was written for (28 BRAINSTORM §8: "mongo fault-delay is the anticipated first" consumer) — deferring it again would leave the anticipated-consumer chain dangling with no natural ROADMAP slot to land it later.

### 2.3 Fault-delay seam: extend the existing halt/resume machinery to persistent + async + post-handoff *(Q2 → ADR-0226)*

**Decision:** Upstream-faithful `StopIteration`-while-delay-pends + `ContinueReading()`-on-timer-fire semantics, delivered by EXTENDING the as-built machinery in three ways (all landing at 29.3 with fault-delay as the consumer):

1. **OnData-halt persistence.** As-built (26.1 `chain.go`): a `StopIteration` from `OnData` stops the current dispatch pass but does NOT persist as a halt across socket reads (only an OnNewConnection `StopIteration` sets the sticky `connHalted`). Extension: a filter must be able to halt from `OnData` such that subsequent reads accumulate in the connection buffer until resumed (upstream `Network::FilterManager` parity).
2. **Cross-goroutine-safe `ContinueReading`.** As-built: `ContinueReading()` is called synchronously (from within filter callbacks on the dispatch goroutine). Extension: the fault-delay timer fires on its own goroutine and must be able to resume the chain safely (synchronization shape pinned at SPEC — anticipated: reuse/extend the ADR-0223 per-connection mutex pattern; the 28.1b two-pump concurrency analysis is the prior art).
3. **Post-handoff read-halt honoring.** As-built (28.1b `readconn.go`): post-terminal-handoff, `readChainConn.Read` feeds bytes through `replayRead` with filter Status IGNORED (the 28.1b SPEC §3.5 observational boundary) — a post-handoff `StopIteration` cannot stop bytes from reaching the upstream. Extension: while the chain is halted, `readChainConn.Read` must NOT return bytes to the terminal's pump (bytes wait; `ContinueReading` unblocks). This LIFTS the §3.5 boundary for halt purposes only; the pure-observation semantics for non-halting filters are unchanged.

`TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM untouched; zero-write-filter and never-halting chains byte-identical (R1-style equivalence argument required at SPEC).

**Rationale:** This is the upstream-faithful semantic, and the user-rejected alternative (a Go-idiomatic blocking sleep inside `OnData`) would leave no reusable seam, depart structurally from upstream's model, and create shutdown/`-race` interactions of its own. The extension shape (semantics-deepening of existing machinery rather than new API) follows the ADR-0219/0221 no-ripple discipline.

**As-built correction (recorded for SPEC accuracy):** the brainstorm initially presented `ContinueReading()` itself as the new seam; code inspection during the brainstorm found it already exists (26.1). The seam is the three extensions above. This correction does not change the Q2 decision's intent (upstream-faithful halt/resume), only its implementation framing.

**Anticipated ADRs:** ADR-0226 (the async halt/resume seam + fault delay + access log; consumes no new interface — semantics extension).

### 2.4 Differential strategy: synthesized bytes + cross-side StatsAsserter *(Q3 → fixture envelope §6)*

**Decision:** Hermetic fixtures with hand-crafted MongoDB wire-protocol bytes (little-endian MsgHeader + BSON documents); chain `[mongo_proxy, tcp_proxy]` on BOTH sides (reference Envoy v1.37.2 + envoy-go); the fixture backend replies with canned OP_REPLY bytes; `StatsAsserter` compares the mirrored counters + gauge cross-side; every assertion proven live via a deliberate-break with `-count=1` (the `reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1` disciplines). Fault-delay arms: 100%-probability fixed delay → `delay_injected` parity (deterministic); the delay duration itself is NEVER compared (BOOTSTRAP §7.2: timing not compared by default). NO real MongoDB server.

**Rationale:** The filter is a passive sniffer — bytes pass through unchanged on both sides, so the stat comparison IS the proof (the phase-28 lesson, unchanged). A real mongod would add a heavy container dependency + nondeterministic driver handshake/heartbeat traffic (`hello`/`isMaster` polling) that breaks exact counter parity.

### 2.5 Stat surface: upstream-parity counters + gauge + envoy-go-strict departures *(self-answered per §9 precedent; SPEC pins roster)*

Upstream-parity roster (anticipated scope `mongo.<stat_prefix>.*` — SPEC empirically pins the exact scope shape + name list against Envoy v1.37.2). Brainstorm-time hypothesis for the fixed roster: `decoding_error`, `delay_injected`, `op_get_more`, `op_insert`, `op_kill_cursors`, `op_query`, `op_query_tailable_cursor`, `op_query_no_cursor_timeout`, `op_query_await_data`, `op_query_exhaust`, `op_query_no_max_time`, `op_query_scatter_get`, `op_query_multi_get`, `op_query_active` (gauge), `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`, `op_command`, `op_command_reply`, `cx_destroy_local_with_active_rq`, `cx_destroy_remote_with_active_rq`, `cx_drain_close` (~22 counters + 1 gauge), plus the dynamic per-command `cmd.<cmd>.total` and per-collection `collection.<collection>.query.total`/`scatter_get`/`multi_get` families (the zookeeper `auth.<scheme>_rq` dynamic-counter precedent), plus the per-collection callsite variant (`collection.<collection>.callsite.<callsite>.query.*` — `$comment` callsite tagging; include-or-defer pinned at SPEC). Upstream's stat macro list may differ from this hypothesis — SPEC pins (D29-3); any upstream counters NOT mirrored land as BEHAVIOR_CONTRACT coverage-boundary records.

### 2.6 Zero new third-party go.mod deps *(self-answered)*

The BSON decode (little-endian length-prefixed documents + typed elements) is implemented in-house (~plain `encoding/binary`); no MongoDB driver dependency. The FaultDelay proto (`envoy.extensions.filters.common.fault.v3.FaultDelay`) is already vendored (used by the phase-09 HTTP fault filter). Matches the 26/27/28 posture.

### 2.7 Dynamic-metadata emission *(per Q1 envelope — in scope at 29.2; SPEC pins shape)*

Upstream mongo_proxy emits per-message dynamic metadata when `emit_dynamic_metadata` is true. envoy-go has the connection-scoped `*dynamicmetadata.Bucket` (ADR-0217; written by rbac shadow at 26.3). Phase 29.2 mirrors the writes (third production write through the bucket). The namespace/key shape + the differential observability strategy (metadata is not directly observable cross-side; anticipated proof: a downstream chain filter or access-log formatter reads it, OR it lands as a unit-test-only surface with a BEHAVIOR_CONTRACT note — the zookeeper AMEND-A9 deferral is the fallback posture) are pinned at SPEC (D29-9).

### 2.8 Mongo access log *(per Q1 envelope — in scope at 29.3; SPEC pins format)*

Upstream mongo_proxy's `access_log` field names a file path receiving per-message log records (this field IS implemented upstream, unlike zookeeper's `[#not-implemented-hide:]` field). envoy-go reuses `internal/accesslog`'s file-sink/async-writer machinery (06.2) with a mongo-specific formatter. The exact upstream record format + the differential comparison strategy (field-mapped log comparison per the 0006-access-log precedent, or counter-only proof + a format unit test) are pinned at SPEC (D29-8).

---

## 3. Framework-survey result — 1 framework-seam extension + 1 NEW filter package + 0 new go.mod deps

### 3.1 EXTENSION: the async halt/resume seam *(per Q2; ADR-0226; lands at 29.3)*

In the existing `internal/filter/network/` package: OnData-halt persistence (chain.go), cross-goroutine-safe `ContinueReading` (chain.go/callbacks.go), post-handoff read-halt honoring (readconn.go). Extends the 26.1 halt machinery + the 28.1b read seam; anticipated zero new exported API. mongo_proxy fault-delay = consumer #1; a future network ext_authz-style pause/resume filter = anticipated consumer #2.

### 3.2 NEW: `internal/filter/network/mongoproxy/` *(per Q0+Q1; ADR-0224/0225; lands across 29.1/29.2/29.3)*

Config parse + BSON parser + request decoder + per-connection active-query list + counters (29.1); response decoder + correlation + gauge + metadata (29.2); fault delay + access log (29.3). Implements BOTH `ReadFilter` and `WriteFilter` (consumer #2 of ADR-0221).

### 3.3 REUSES

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b) — ReadFilter + WriteFilter chains, Buffer, registry, builtins seam, chainRuntime, `readChainConn`/`writeChainConn`, the existing `ContinueReading`/`connHalted` machinery (extended per §3.1).
- `internal/stats/` (06.1) — counters + **gauges** (`gauge.go`) + `NewCounterIfAbsent` dynamic-name convention; the `internal/stats/name.go` Prometheus inline-prefix arm pattern (ADR-0138; `.rbac.`/`.zookeeper.` precedents).
- `internal/accesslog/` (06.2) — file sink + async writer for the mongo access log (mongo-specific formatter is new).
- `internal/dynamicmetadata/` connection-scoped Bucket (22.2/26.3, ADR-0217) — the `emit_dynamic_metadata` writes.
- `internal/filter/http/fault/` (09) — FaultDelay proto parse/evaluation precedent (percentage + fixed delay semantics).
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; untouched by 29.
- The differential harness + `StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints).
- `envoy.extensions.filters.network.mongo_proxy.v3` + `envoy.extensions.filters.common.fault.v3` proto bindings (go-control-plane v1.32.4 — verified present).

---

## 4. Per-route applicability — none (network filters are not route-scoped)

Per the 26 BRAINSTORM §4 confirmation: network filters carry no `typed_per_filter_config` surface. Not applicable to phase 29.

---

## 5. Stat surface hypothesis

### 5.1 29.1 (requests; SPEC pins)

~13 fixed request-side counters (`op_query` + the 6 `op_query_*` flag/shape counters + `op_insert`/`op_get_more`/`op_kill_cursors`/`op_command` + `decoding_error`) + the dynamic `cmd.<cmd>.total` + `collection.<collection>.query.total`/`scatter_get`/`multi_get` families (fixture corpus exercises a pinned set of commands/collections). Anticipated +15–25 differentially-mirrored names.

### 5.2 29.2 (responses + correlation; SPEC pins)

~7 fixed response/connection counters (`op_reply` + 3 reply-flag counters + `op_command_reply` + 2 `cx_destroy_*` counters) + the `op_query_active` GAUGE. Anticipated +8–12.

### 5.3 29.3 (fault delay + access log + drain; SPEC pins)

`delay_injected` + `cx_drain_close`. Anticipated +2.

### 5.4 Project stat count delta

337 → **~360–380** at family-row-done (SPEC pins the exact mirrored roster; the dynamic cmd/collection families count by the BEHAVIOR_CONTRACT convention established for zookeeper's `auth.<scheme>_rq`). Any upstream counters NOT mirrored land as BEHAVIOR_CONTRACT coverage-boundary records.

### 5.5 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The per-command + per-collection `reply_num_docs`/`reply_size`/`reply_time_ms` HISTOGRAM families unmirrored (ADR-0060); upstream runtime-key gating (`mongo.proxy_enabled`, `mongo.logging_enabled`, `mongo.connection_logging_enabled`, `mongo.drain_close_enabled`, the fault runtime keys) — envoy-go has no runtime layer → the filter behaves as if all keys return their defaults (departure recorded; the Runtime family row stays the future home); the `$comment` callsite stat family include-or-defer (D29-3).

---

## 6. Differential fixture envelope — anticipated four directories across 3 sub-phases

### 6.1 29.1 fixtures (+2)

- **`0049-mongo-requests`** (cross-side): chain `[mongo_proxy, tcp_proxy]` both sides; driver sends hand-crafted OP_QUERY (a `$cmd` command + a regular collection query with flag bits + a scatter_get-shaped and a multi_get-shaped query) + OP_INSERT + OP_GET_MORE + OP_KILL_CURSORS byte sequences; backend = canned-bytes responder; `StatsAsserter` compares the mirrored request-side counters incl. the `cmd.*`/`collection.*` dynamic names; one garbage-bytes arm proves `decoding_error` + passthrough-not-broken; deliberate-break liveness proof recorded with `-count=1`.
- **`0050-mongo-boot-reject`** (boot-reject; separate dir per the fixture-dispatch constraint): missing `stat_prefix` → both sides reject at boot.

### 6.2 29.2 fixtures (+1)

- **`0051-mongo-responses`** (cross-side): query→OP_REPLY round trips; `StatsAsserter` on `op_reply` + the reply-flag counters + the `op_query_active` gauge (asserted at a quiesced point: all queries answered → 0; an unanswered-query arm → 1) + the `cx_destroy_*_with_active_rq` counters.

### 6.3 29.3 fixtures (+1)

- **`0052-mongo-fault-delay`** (cross-side): 100%-probability fixed delay on both sides → `delay_injected` parity + the delayed traffic still completes (passthrough proof); a no-delay arm proves the seam does not perturb the non-faulted path. Whether the access-log proof is an arm of this fixture or needs its own dir (`0053`) is pinned at SPEC (D29-8) — if upstream's record format is timing-bearing, the access-log proof falls back to unit tests + a coverage-boundary note.

### 6.4 Total

50 → **54** (55 if D29-8 adds an access-log dir). SPEC pins exact numbering + arm rosters.

### 6.5 No conformance harness

No new conformance harness (matches 26/27/28). The h2spec + proxy-wasm gates re-run asserted-unaffected at each sub-phase six-gate.

---

## 7. Anticipated ADRs — ~3 ADRs (ADR-0224 .. ADR-0226)

Next-free ADR at master tip is **ADR-0224** (DECISIONS.md tail ADR-0223; the ADR-0209 escape-valve reserve stands unconsumed).

- **ADR-0224** *(29.1)* — the `mongo_proxy` filter, request side: 5-field config parse, the in-house BSON parser, the little-endian wire decoder, per-opcode/per-command/per-collection request counters, the active-query list, the 8th built-in, the `.mongo.` prom arm.
- **ADR-0225** *(29.2)* — the response side: OP_REPLY decode, requestID↔responseTo correlation, the `op_query_active` gauge (the project's first mirrored gauge), `cx_destroy_*` counters, dynamic-metadata emission.
- **ADR-0226** *(29.3)* — the async halt/resume seam extension (OnData-halt persistence + cross-goroutine ContinueReading + post-handoff read-halt) + fault-delay injection + the mongo access log + `cx_drain_close` + the parent-29 ROLLUP.

The SPEC may re-allocate (e.g. split the seam extension into its own number → 4 ADRs, next-free ≈ ADR-0228); anticipated default is 3 (next-free after phase 29 phase-done ≈ **ADR-0227**). §Context drafts land at the parent/sub-phase SPECs; §Decision/§Consequences bodies at each IMPL per ADR-0044.

---

## 8. Deferred items

- **The `reply_*` histogram families** (per-command + per-collection `reply_num_docs`/`reply_size`/`reply_time_ms`) — deferred per ADR-0060; coverage-boundary record at 29.2.
- **OP_MSG / modern MongoDB protocol** — mirrored ONLY to upstream v1.37.2's actual decode envelope (empirical pin D29-4); if upstream is legacy-protocol-only, OP_MSG decode is explicitly out of scope (upstream parity, not a gap).
- **Runtime-key gating** (`mongo.proxy_enabled`, `mongo.logging_enabled`, `mongo.connection_logging_enabled`, `mongo.drain_close_enabled`, fault runtime keys) — no runtime layer exists; the filter behaves at key defaults (envoy-go-strict departure; the Runtime + hot restart family row is the future home).
- **Real-MongoDB-server integration fixtures** — out of scope; hermetic synthesized bytes only.
- **`$comment` callsite stats** (`collection.<c>.callsite.<cs>.query.*`) — include-or-defer pinned at SPEC (D29-3).
- **The remaining protocol proxies** — `redis`, `kafka_broker`, `thrift` — each its own future family phase, each needing its own brainstorm-time risk assessment (terminal-proxy seams; contrib-image risk).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **28 BRAINSTORM §8 / ADR-0221 §Consequences** ("WriteFilter halt/buffer semantics — documented-unsupported until a consumer needs it (mongo fault-delay is the anticipated first)" + "anticipated #2 `mongo_proxy`") — **CONSUMED by phase 29.** This is the explicit closure pickup. Note the as-built correction: upstream mongo's fault-delay halts the READ path (`onData` returns StopIteration while the timer pends), not the write path — the anticipated consumer arrives, but the halt seam it needs is the read-side async-resume extension (§2.3), not write-path StopIteration. The WriteFilter consumption is the ordinary both-direction decode (OP_REPLY in `OnWrite`), which needs no halt. ADR-0226 records this refinement against the ADR-0221 anticipation.
- **28.1b SPEC §3.5** (post-handoff observational boundaries: replayRead ignores Status) — **PARTIALLY LIFTED by phase 29.3** (for halt purposes only; pure observation unchanged).
- **Zookeeper AMEND-A9** (dynamic-metadata deferral) — phase 29.2 lands the family's first protocol-filter metadata emission; whether this creates a retroactive pickup obligation for zookeeper metadata is OUT of phase-29 scope (recorded as a §10 SPEC question, D29-9).
- **26.3 D-26.3-4** (rbac SNI/mTLS arms differential gap) — not picked up here; carried.
- **`tcp_proxy` `downstream_cx_*` family unmirrored** (phase-27 record) — not picked up here; carried.

---

## 10. BRAINSTORM-time open questions for parent-SPEC-time resolution (empirical pins against Envoy v1.37.2 per ADR-0004)

The parent SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25/26/27/28 SPEC precedent) against Envoy v1.37.2 source + the standard reference image + go-control-plane v1.32.4 bindings:

- **D29-1** *(SPEC-BLOCKING for Q3)* — confirm the standard `envoyproxy/envoy:v1.37.2` reference image ships `mongo_proxy` (boot a listener with it; it is a core extension at `source/extensions/filters/network/mongo_proxy`, not contrib — verify, since this kills-or-keeps the cross-side strategy).
- **D29-2** — exact proto field roster + PGV constraints + defaults (`stat_prefix` required; `delay` FaultDelay percentage/fixed_delay semantics + PGV; `commands` list semantics — does it gate which commands get `cmd.*` stats, and what is the default command set?).
- **D29-3** — the exact stat scope + naming Envoy v1.37.2 emits (`mongo.<stat_prefix>.*` hypothesis) + the full fixed-counter roster vs the §2.5 hypothesis + which are eager vs lazy + the dynamic `cmd.*`/`collection.*` naming rules + the `$comment` callsite family (include-or-defer).
- **D29-4** — MongoDB wire-protocol framing pins: little-endian MsgHeader layout; which opcodes upstream v1.37.2 decodes (OP_QUERY/OP_REPLY/OP_GET_MORE/OP_INSERT/OP_KILL_CURSORS/OP_COMMAND/OP_COMMANDREPLY; **does it decode OP_MSG?** — the largest unknown); per-opcode structure layouts; the BSON subset needed (command-name extraction = first element key of a `$cmd` query document; collection extraction from `fullCollectionName`; `scatter_get`/`multi_get` heuristics [`_id` presence / `$in`]; `$maxTimeMS`; `$comment`); enough to hand-craft the fixture byte corpora + the decoder contract. Per `reference_wire_format_both_sides_see_same_bytes`: adopt upstream's framing verbatim — both sides see the same bytes.
- **D29-5** — `decoding_error` semantics: what upstream counts as a decode error; passthrough behavior on decode failure (the filter must NEVER break the connection); partial-message reassembly semantics.
- **D29-6** — fault-delay semantics: when the delay is evaluated (per connection? per query? on which decode events?); the FaultDelay percentage semantics (numerator/denominator); `delay_injected` increment timing; what exactly is delayed (further reads only? the already-buffered bytes?); interaction with connection close during an active delay.
- **D29-7** — upstream `continueReading()` re-dispatch semantics (which filter the iteration resumes at; what happens to bytes buffered during the halt) — to shape the §2.3 seam extension faithfully + the equivalence argument for never-halting chains.
- **D29-8** — the mongo access-log record format (upstream `MONGO_ACCESS_LOG` / per-message log lines) + rotation/flush semantics + the differential comparison strategy (field-mapped log diff per the 0006-access-log precedent, or unit-test-only + coverage boundary if the format is timing-bearing).
- **D29-9** — dynamic-metadata emission shape (namespace + key roster + value types) + the differential observability strategy + whether a retroactive zookeeper-metadata pickup obligation is created.
- **D29-10** — `cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq`/`cx_drain_close` semantics: the local-vs-remote close-direction distinction (does the as-built `Connection`/`OnDestroy` surface expose close direction? if not: small seam or coverage boundary) + the drain-decision integration with the phase-08.2 drain machinery.
- **D29-11** — the per-sub-phase task/LoC envelope confirming each sub-phase fits the ADR-0045 gate (re-checked again at each PLAN).
- **D29-12** — fuzzer envelope: the wire+BSON request-decode fuzzer at 29.1 (39th) confirmed; whether 29.2 adds a response-decode fuzzer (40th) or folds response decode into one fuzzer.

---

## 11. Prior-phase lessons applied

- **Differential liveness must be proven** (`reference_differential_asserter_dispatch`; the 0030 dead-assertion lesson). Applied: the StatsAsserter IS the load-bearing proof; every fixture records a deliberate-break.
- **Deliberate-break verification needs `-count=1`** (`reference_differential_break_protocol_count1`; the 28.1b Task-7 lesson). Applied: every R4 break protocol in phase 29 runs with `-count=1`.
- **Cross-side XOR boot-reject per fixture dir** (`reference_differential_fixture_dispatch_constraint`). Applied: `0049`/`0051`/`0052` cross-side; `0050` boot-reject; never mixed.
- **Wire-format pins: both sides see the same bytes** (`reference_wire_format_both_sides_see_same_bytes`; the 28.2 Task-3 watch-event lesson). Applied: ALL mongo framing/BSON layouts are empirical pins against upstream v1.37.2 (D29-4); "our frame format" is never a valid deviation.
- **TypeURL via `proto.MessageName`, never the docs string** (`reference_network_filter_typeurl_extensions`). Applied: pinning-test at 29.1 Task 1.
- **OnNewConnection must Continue** (`reference_network_read_filter_onnewconnection_halts`). Applied: mongo_proxy's OnNewConnection is a no-op Continue; all decode work in OnData/OnWrite; the fault-delay halt is an OnData halt (per upstream), never an OnNewConnection halt.
- **Observer filters must implement WriteFilter to get the read seam** (`reference_network_chain_terminal_handoff_ends_ondata`). Applied: mongo_proxy implements both directions natively (OP_REPLY decode lives in OnWrite), so the wrap predicate is satisfied intrinsically.
- **No-ripple seam additions** (ADR-0219/0221: thread state via ctx/wrappers, never signature churn). Applied: the §2.3 seam extension changes semantics of existing machinery; anticipated zero new exported API; `TerminalFilter.Handle` + all fakes untouched.
- **Defer-with-allowance, consume-at-consumer** (ADR-0213 → ADR-0221 at 28; ADR-0221 §Consequences → ADR-0226 here). Applied: the halt/resume extension lands at 29.3 WITH fault-delay, never speculatively.
- **Proto roster extraction needs digit-inclusive regexes** (`reference_proto_roster_extraction_digits`). Applied: any opcode/command roster extraction at SPEC time uses digit-inclusive patterns.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`). Applied at every IMPL.

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 29 = `mongo_proxy`, the fourth §9 Network-filters row — consumer #2 of the ADR-0221 WriteFilter seam as anticipated; (Q1) FULL counter+gauge parity — both-direction op/cmd/collection counters + the `op_query_active` gauge + fault-delay + access log + dynamic metadata; histograms deferred per ADR-0060; 3-way FEATURE-PROGRESSIVE pre-split (29.1 wire+BSON+requests / 29.2 responses+correlation / 29.3 seam+fault-delay+access-log); (Q2) the upstream-faithful async halt/resume seam delivered as an EXTENSION of the existing 26.1/28.1b machinery (OnData-halt persistence + cross-goroutine ContinueReading + post-handoff read-halt honoring — the as-built-correction finding of this brainstorm); (Q3) hermetic synthesized-byte fixtures with cross-side `StatsAsserter` as the load-bearing proof + deterministic 100%-probability fault-delay arms. Self-answered per §9 precedent: upstream-parity stat roster + envoy-go-strict departures (histograms, runtime keys); zero new go.mod deps (in-house BSON); dynamic metadata + access log in scope with SPEC-pinned shapes. One framework-seam extension (async halt/resume) + one NEW filter package (`mongoproxy`). Anticipated 3 ADRs (ADR-0224..0226), fixtures 50 → 54(-55), stat surface 337 → ~360–380, fuzzers 38 → 39–40.

The next session authors the parent SPEC (`superpowers:writing-plans` scoped to parent-SPEC authoring per the 22/24/25/26/28 parent-row precedent), executing the §10 D29-1..D29-12 empirical pins IN-SESSION against Envoy v1.37.2 per ADR-0004, anchoring the ADR-0224/0225/0226 §Context drafts, and formalizing the 3-way split surface-mapping. Per ADR-0106, parent row 29 registers `in-progress` with sub-phases listed at this BRAINSTORM-DONE commit; sub-rows 29.1 + 29.2 + 29.3 register `planned`.
