# Phase 29 — `mongo_proxy` + the async halt/resume seam (parent master SPEC)

> **For agentic workers:** this is the PARENT SPEC for the phase-29 3-way pre-split (29.1 / 29.2 / 29.3). It is NOT directly executable. Per the phase-22 / phase-24 / phase-25 / phase-26 / phase-28 parent-row precedent, each sub-phase lands its own SPEC → PLAN → IMPL in dedicated sessions. This parent SPEC: (1) resolves the BRAINSTORM §10 D29-1..D29-12 empirical pins IN-SESSION against reference Envoy v1.37.2 + go-control-plane v1.32.4 (§11), (2) formalizes the 3-way split surface-mapping + per-sub-phase scope boundaries (§3), and (3) anchors the phase-29 ADR §Context drafts ADR-0224..ADR-0226 (§10). The next session, per BOOTSTRAP §5 + the per-sub-phase precedent, authors the **29.1 SPEC**.

**Goal:** Land `envoy.filters.network.mongo_proxy` — a passive both-direction MongoDB legacy-wire-protocol observability sniffer — at FULL counter+gauge parity (histograms deferred per ADR-0060) PLUS fault-delay injection, the mongo access log, and dynamic metadata, across a 3-way feature-progressive pre-split; and land the **async halt/resume seam** (the framework's fifth structural extension) with fault-delay as its first consumer.

**Architecture:** A NEW `internal/filter/network/mongoproxy/` package implements BOTH `ReadFilter` and `WriteFilter` (consumer #2 of the ADR-0221 conn-wrap seam, exactly as anticipated): an in-house little-endian BSON parser + the MongoDB legacy wire decoder (the EXACTLY-7-opcode upstream envelope — OP_MSG is NOT decoded, §11.4/AMEND-B5), emitting the 23-stat fixed roster under scope `mongo.<stat_prefix>.` + the dynamic `cmd.*`/`collection.*`/callsite counter families. The existing `internal/filter/network/` framework gains the async halt/resume extension at 29.3: ACTIVE asynchronous `ContinueReading` (cross-goroutine-safe) + post-handoff read-halt honoring in `readChainConn`/`replayRead` — `TerminalFilter.Handle` UNCHANGED, zero new exported API anticipated, never-halting chains byte-identical. Cross-side `StatsAsserter` counter+gauge parity is the load-bearing differential proof (the filter never mutates bytes — a body differential is vacuous).

**Tech Stack:** Go 1.26.2; go-control-plane v1.32.4 proto bindings (ADR-0008); reference Envoy v1.37.2 (ADR-0008); `internal/stats/` counters + gauges (06.1); `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b); `internal/accesslog/` (06.2); `internal/dynamicmetadata/` (ADR-0217); the differential harness + `StatsAsserter`. ZERO new third-party `go.mod` dependencies (BSON decode is plain `encoding/binary` little-endian reads; the `$comment` callsite JSON parse is stdlib `encoding/json`).

**Authored:** 2026-06-03. **Empirical-pin probe date:** 2026-06-03.

---

## 1. Mission summary

Phase 29 is the **FOURTH §9 Network-filters-family row** (after the phase-26 family-parent, the phase-27 `sni_cluster` flat row, and the phase-28 `zookeeper_proxy` parent row) and a parent pre-split row per ADR-0106 (the project's FIFTH BRAINSTORM-time pre-split, after 22/25/26/28). It delivers two structurally coupled things:

1. **`envoy.filters.network.mongo_proxy`** — a passive observability sniffer that decodes the MongoDB legacy wire protocol (little-endian MsgHeader + BSON) in BOTH directions and emits per-opcode + per-command + per-collection counters plus the `op_query_active` GAUGE (the project's first differentially-mirrored gauge), the mongo access log, and dynamic metadata. It is the family's second stats-PRIMARY filter and **consumer #2 of the ADR-0221 `network.WriteFilter` seam** — exactly the consumer that ADR's §Consequences anticipated.
2. **The async halt/resume seam** — the framework's FIFTH structural extension (after the TerminalFilter seam at 26.2, the override seam at 27, the WriteFilter seam at 28.1a, and the read seam at 28.1b): ACTIVE asynchronous `ContinueReading` + cross-goroutine safety + post-handoff read-halt honoring, consumed by mongo fault-delay injection (the ADR-0221 §Consequences "mongo fault-delay is the anticipated first halt consumer" closure pickup, REFINED to the read side per upstream `onData` semantics — §4.1/AMEND-B13).

The design was settled at BRAINSTORM via a 4-question user dialogue (`docs/envoy-go/phases/29-network-filter-mongo-proxy/BRAINSTORM.md` §2): Q0 subject = `mongo_proxy`; Q1 FULL counter+gauge parity minus histograms + 3-way pre-split; Q2 upstream-faithful async halt/resume as an extension of the as-built 26.1/28.1b machinery; Q3 hermetic synthesized-byte fixtures + cross-side `StatsAsserter`. This SPEC does NOT re-litigate those decisions; it executes the empirical pins they deferred and formalizes the surface-mapping.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D29-1..D29-12 scrape against Envoy v1.37.2 + go-control-plane v1.32.4 + the live reference image CONFIRMED the SPEC-blocking hypothesis (D29-1) and REFINED/REFUTED several others. The load-bearing amendments to the BRAINSTORM design, each carried into the relevant §§ below:

- **AMEND-B1 (D29-1/D29-3 — stat scope CONFIRMED `mongo.<stat_prefix>.<counter>`).** Upstream `config.cc:24` builds `fmt::format("mongo.{}", proto_config.stat_prefix())` — the literal token `mongo` is the scope ROOT, the prefix is the MIDDLE segment. Confirmed live with two stat_prefix values (`mongo.mongoprobe.op_query`, `mongo.otherprefix.decoding_error`). The BRAINSTORM's `mongo.<stat_prefix>.*` hypothesis stands (in contrast to phase 28, where the analogous hypothesis was reversed). See §7.1.
- **AMEND-B2 (D29-1 — Prometheus exposition is TAG-EXTRACTED, not inline-prefix; REFUTES the BRAINSTORM's `.mongo.` ADR-0138 arm hypothesis).** Reference Envoy's `/stats/prometheus` hoists the stat_prefix into a **label**: `envoy_mongo_op_query{envoy_mongo_prefix="mongoprobe"} 0` — the metric name is flat `envoy_mongo_<leaf>` and the prefix never appears in the name (probed live, two prefixes; one `# TYPE` line per metric; the gauge emits `# TYPE envoy_mongo_op_query_active gauge`). This is the exact OPPOSITE of the phase-28 finding (zookeeper: flat inline-prefix, no labels): the correct precedent for the `internal/stats/name.go` arm is the **`.rbac.` TAG-EXTRACTOR shape (ADR-0218)**, not the ADR-0138 inline-prefix shape the BRAINSTORM (and ROADMAP row 29.1) hypothesized. See §7.4.
- **AMEND-B3 (D29-1/D29-3 — the fixed roster is EXACTLY 22 counters + 1 gauge; `delays_injected` is PLURAL; NO histograms in the macro).** Upstream's `ALL_MONGO_PROXY_STATS(COUNTER, GAUGE, HISTOGRAM)` macro (`proxy.h:49-72`) declares 22 counters + 1 gauge (`op_query_active`, Accumulate) and ZERO histograms (the HISTOGRAM macro arg is declared but never used — all mongo histograms live in the DYNAMIC `MongoStats` families, §7.3). The BRAINSTORM's `delay_injected` hypothesis is corrected to **`delays_injected`**. Project stat surface goes 337 → **360** (+23 — the exact roster, §7.2). See §7.
- **AMEND-B4 (D29-1/D29-3 — fixed-stat creation is per-FILTER-INSTANCE, i.e. per CONNECTION; the reference admin shows ZERO mongo stats until the first downstream connection).** Upstream `generateStats` runs in the `ProxyFilter` constructor (`proxy.cc:64`), and a `ProxyFilter` is constructed per connection (`config.cc:47` `addFilter` inside the per-connection factory callback) — pool-deduped by the scope so names collapse. Confirmed live: `/stats | grep mongo` is EMPTY at boot; the full 23-name roster materializes (all at 0) after the first downstream TCP connection. Differential consequence: fixture stat assertions MUST be post-first-connection; envoy-go's creation posture (eager-at-config-parse vs first-connection) is D-P1. See §7.2.
- **AMEND-B5 (D29-4 — the decode envelope is EXACTLY 7 opcodes; OP_MSG is NOT decoded; the largest unknown resolves to LEGACY-ONLY).** The codec dispatch switch (`codec_impl.cc:372-426`) decodes exactly: **Reply(1), Insert(2002), Query(2004), GetMore(2005), KillCursors(2007), Command(2010), CommandReply(2011)**. Everything else — the modern **OP_MSG(2013)**, the legacy Msg(1000), Update(2001), Delete(2006) — hits the `default` branch → `EnvoyException("invalid mongo op N")` → `decoding_error`. Per `reference_wire_format_both_sides_see_same_bytes`, envoy-go mirrors EXACTLY this envelope: 7 decoded opcodes, throw-equivalent on everything else. OP_MSG decode is explicitly out of scope (upstream parity, not a gap). See §11.4.
- **AMEND-B6 (D29-5 — decode error STOPS sniffing for the connection LIFETIME; ≠ zookeeper's abandon-buffer-keep-decoding).** On any decode `EnvoyException`, upstream increments `decoding_error` ONCE and sets `sniffing_ = false` (`proxy.cc:340-345`) — from then on the filter drains its private buffers without decoding, for the remainder of the connection. No per-error close; passthrough unaffected (the filter only ever copies the chain bytes). This differs structurally from zookeeper's abandon-current-buffer-keep-decoding model and refines the BRAINSTORM §6.1 garbage-bytes arm: after the garbage arm fires, NO further decode happens on that connection (the fixture must use a fresh connection per arm). See §11.5.
- **AMEND-B7 (D29-3 — `commands` semantics: builtin-name REMEMBERING (cardinality control), NOT emission gating; default `{delete, insert, update}`; command-name aliasing).** The configured `commands` list controls which command names are pre-remembered as stat-name builtins; a decoded command NOT in the list still emits a counter, but bucketed as `cmd.unknown_command.total` instead of `cmd.<name>.total` (`mongo_stats.cc:29-31` + `proxy.cc:147-153`). An explicit list REPLACES the default entirely. Additionally the decoder NORMALIZES wire command names before stats: `find` → handled as a query (collection path); `collstats`→`collStats`, `dbstats`→`dbStats`, `findandmodify`→`findAndModify`, `getlasterror`→`getLastError`, `ismaster`→`isMaster` (`utility.cc:21-37`). See §11.3.
- **AMEND-B8 (D29-6/D29-7 — fault-delay + continueReading semantics pinned).** The delay is evaluated **per decoded request-direction message** (`tryInjectDelay()` at the top of decodeQuery/Insert/GetMore/KillCursors/Command/CommandReply — NOT decodeReply), with a re-entrancy guard (an armed timer suppresses re-evaluation). While the timer pends, `onData` returns `StopIteration` on EVERY read (`proxy.cc:378-383`) — upstream re-dispatches the full read chain from filter 0 on each fresh socket read, so the "halt" is simply the filter repeatedly returning StopIteration, NOT a filter-manager-level parked state. `delays_injected` increments at timer-ARM time. Timer fire → `delay_timer_.reset()` + `read_callbacks_->continueReading()`, which resumes iteration at the filter AFTER mongo_proxy (`filter_manager_impl.cc:75` `std::next(filter->entry())`) with the connection's accumulated read buffer — the halting filter is NOT re-invoked by the resume. Socket reads continue during the halt (mongo never calls readDisable); connection close during a pending delay cancels the timer. See §11.6/§11.7.
- **AMEND-B9 (D29-2 — FaultDelay PGV: the oneof is REQUIRED; `fixed_delay` must be > 0s; TWO new boot-reject parity arms).** `FaultDelay.fault_delay_secifier` (upstream's typo, preserved in the wire format) is a PGV-REQUIRED oneof — `delay: {}` is a boot-reject; the `fixed_delay` arm must be `> 0s` — `delay: {fixed_delay: 0s}` is a boot-reject. `percentage` is optional (FractionalPercent; default numerator 0 / denominator HUNDRED). See §5.3/§6.
- **AMEND-B10 (D29-8 — the access log is timing-bearing JSON → differential log comparison NOT viable; unit-test + coverage-boundary fallback fires).** Upstream writes one JSON line per decoded message (BOTH directions): `{"time": "<wall-clock>", "message": <message.toString(full)>, "upstream_host": "<addr|->"}` (`proxy.cc:48-56`), via the AccessLogManager file API. The `time` field is a per-message wall-clock timestamp → the record format is timing-bearing → the BRAINSTORM's anticipated fallback fires: the access-log proof is format unit tests + a BEHAVIOR_CONTRACT coverage-boundary note; NO access-log fixture dir (fixture count stays 54, not 55). Also: envoy-go's `internal/accesslog.AsyncFileSink` hard-wires the HTTP `Default` formatter (`writer.go:88`) — 29.3 needs a formatter seam (D-P7). See §11.8.
- **AMEND-B11 (D29-9 — dynamic-metadata shape pinned; mirrorable; differential-invisible).** Namespace `envoy.filters.network.mongo_proxy`; the value is a Struct keyed by **collection name** (resource) → ListValue of operation strings; only `"insert"` and `"query"` operations are emitted in v1.37.2 (`update`/`delete` keys defined but TODO-unused); the Struct is CLEARED at the top of every `doDecode` pass. This maps directly onto the ADR-0217 `*dynamicmetadata.Bucket` (`Set(filterName, key, value)`). The emission has zero differential observability (no cross-side surface) → mirrored at 29.2 with unit-test proof + a coverage note (D-P11). NO retroactive zookeeper-metadata pickup obligation is created (zookeeper's emission requires deep payload parsing that stays deferred under AMEND-A9; mongo's emission consumes only fields the decoder already extracts). See §11.9.
- **AMEND-B12 (D29-10 — `cx_destroy_*_with_active_rq` needs close-DIRECTION; an as-built framework gap).** Upstream keys the two counters on `Network::ConnectionEvent::LocalClose` vs `RemoteClose` delivered to `onEvent` while `active_query_list_` is non-empty. envoy-go's as-built framework does NOT expose close direction to network filters (`OnDestroy()` carries no event/reason). 29.2 needs either a small close-direction accessor (anticipated; the close-type recording machinery at `chain.go` is the natural anchor) or a coverage boundary — D-P4. See §11.10.
- **AMEND-B13 (D29-6/D29-7 + as-built — the seam is REFRAMED: upstream has NO persistent filter-manager halt either; the extensions are ACTIVE-ASYNC-RESUME + CROSS-GOROUTINE SAFETY + POST-HANDOFF HONORING).** Upstream's fault-delay "halt" is just the filter returning StopIteration on every read while its timer pends (AMEND-B8); the filter IS re-invoked per read. envoy-go's existing per-pass-stop semantics (`chain.go:367-370` — OnData StopIteration leaves `resumeIdx` at the filter; the next read re-dispatches it) ALREADY mirror this pre-handoff, including handoff deferral (`terminalReady()` is false while `resumeIdx` is parked). What is actually missing — the three REAL extensions, refining BRAINSTORM §2.3: (i) **ACTIVE asynchronous `ContinueReading`** — today's `ContinueReading()` called outside the connHalted/re-entrant contexts merely sets `resumeRequested`, which nothing consumes until the next socket read; the timer goroutine's resume must actively advance past the halting filter, dispatch the buffered bytes to the remaining chain, and re-check terminal readiness; (ii) **cross-goroutine safety** — `ContinueReading` from a `time.AfterFunc` goroutine races the read-loop goroutine (pre-handoff) or pump-A's `replayRead` (post-handoff) on unlocked `chainRuntime` state; (iii) **post-handoff read-halt honoring** — `replayRead` (`chain.go:389-395`) ignores Status entirely and `readChainConn.Read` returns bytes to the terminal's pump regardless; while a filter is halted post-handoff, those bytes must be withheld and released on resume (lifting the 28.1b §3.5 boundary for halt purposes only). See §4.1.

### 1.2 ADR continuity + D-hypothesis disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0223**; next-free **ADR-0224**. This SPEC anchors the phase-29 §Context drafts ADR-0224..ADR-0226 (three ADRs, locked at BRAINSTORM §7; §Decision/§Consequences bodies land at each sub-phase IMPL per ADR-0044). No ADR number is consumed at SPEC time beyond the §Context drafts. The ADR-0209 escape-valve reserve carried from the §9 family STANDS-UNCONSUMED. All twelve D29 pins are resolved at this session (§11); the remaining open items are sub-phase-SPEC/PLAN D-questions (§12), not empirical pins.

---

## 2. Scope — non-purposes + REUSES-not-consumed

### 2.1 Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The dynamic HISTOGRAM families** (`cmd.<cmd>.reply_num_docs`/`reply_size`/`reply_time_ms` + the per-collection and per-callsite variants) — deferred per ADR-0060 (project-wide histogram deferral); BEHAVIOR_CONTRACT coverage-boundary record at 29.2. NOTE (AMEND-B3): there are NO fixed-roster histograms — every mongo histogram is a dynamic-family member, so the deferral excises the `reply_*` suffixes from the dynamic families while keeping their counter siblings (`cmd.<cmd>.total`, `collection.<c>.query.*`).
- **OP_MSG / modern MongoDB protocol decode** — upstream v1.37.2 does NOT decode OP_MSG (AMEND-B5); envoy-go mirrors the exactly-7-opcode envelope. Explicitly upstream parity, not a gap.
- **Runtime-key gating** (`mongo.proxy_enabled`, `mongo.logging_enabled`, `mongo.connection_logging_enabled`, `mongo.drain_close_enabled`, `mongo.fault.fixed_delay.percent`, `mongo.fault.fixed_delay.duration_ms`) — envoy-go has no runtime layer; the filter behaves at key defaults (all featureEnabled keys default 100% = enabled; the fault keys default to the configured proto values). Recorded as an envoy-go-strict departure; the Runtime family row stays the future home. See §11.3 item 6.
- **Real-MongoDB-server integration fixtures** — out of scope; hermetic synthesized bytes only (Q3). A real mongod/driver would speak OP_MSG (which neither side decodes) and add nondeterministic handshake traffic.
- **Differential access-log comparison** — NOT viable (timing-bearing format, AMEND-B10); the access log lands at 29.3 with unit-test proof + coverage boundary.
- **The `update`/`delete` dynamic-metadata operations** — upstream defines the keys but never emits them (TODO at `proxy.cc:30`); envoy-go mirrors the emitted set only (`insert` + `query`).
- **`injectWriteDataToFilterChain` / `disableClose` WriteFilterCallbacks surface** — stays deferred under the ADR-0221 API-revision allowance (mongo needs neither).
- **The remaining protocol proxies** (`redis`, `kafka_broker`, `thrift`) — each its own future family phase, each needing its own brainstorm-time risk assessment.

### 2.2 REUSE-by-absence: no per-route surface

Network filters carry no `typed_per_filter_config` surface (phase-26 parent SPEC §2.2 confirmation; re-confirmed by absence — the MongoProxy proto has no `*PerRoute` message, §11.3). The ADR-0125 roster is untouched.

### 2.3 REUSES (not new primitives)

- `internal/filter/network/` (26.1/26.2/27/28.1a/28.1b) — ReadFilter + WriteFilter chains, drainable `Buffer` + `TotalAppended`, freeze-after-boot `*Registry`, `chainRuntime`, `readChainConn`/`writeChainConn`/`prefixConn`, the existing `ContinueReading`/`connHalted`/per-pass-stop machinery (EXTENDED per §4.1), `builtins.RegisterBuiltins`.
- `internal/stats/` (06.1) — counters + **gauges** (`gauge.go:32-60` — `Inc`/`Dec`/`Load`, lock-free atomics, exists since 06.1, first differential consumer is 29.2) + `NewCounterIfAbsent` dynamic-name convention; `internal/stats/name.go` (the new `mongo.` TAG-EXTRACTOR arm follows the `.rbac.` ADR-0218 shape per AMEND-B2).
- `internal/accesslog/` (06.2) — `AsyncFileSink` (bounded-channel async writer, drop-newest backpressure) for the mongo access log; the formatter seam is 29.3's (D-P7).
- `internal/dynamicmetadata/` connection-scoped `Bucket` (22.2/26.3, ADR-0217) — the `emit_dynamic_metadata` writes (third production write through the bucket).
- `internal/filter/http/fault/` (09) — FaultDelay parse/eval precedent: `percentageToFloat` (FractionalPercent → [0,100]), `rollPercent`, `time.AfterFunc` delay timing (`fault.go:120-127, 216-230, 374-382`).
- `internal/filter/network/zookeeperproxy/` (28) — the consumer-#1 package shape mongoproxy mirrors (two-step factory, decoder-internal reassembly, `chainConsumed` high-water tracking, the ADR-0223 per-connection mutex pattern for cross-goroutine correlation state).
- `internal/filter/tcpproxy/` (02/26.2/27) — the terminal in every fixture chain; untouched by 29.
- The differential harness + `fixture.StatsAsserter` (+ the fixture-dispatch + asserter-dispatch + `-count=1` break-protocol memory constraints).
- `envoy.extensions.filters.network.mongo_proxy.v3` + `envoy.extensions.filters.common.fault.v3` + `envoy.type.v3.FractionalPercent` proto bindings — already vendored (go-control-plane v1.32.4); the mongo_proxy blank-import added to `internal/bootstrap/bootstrap.go`.
- The freeze-after-boot registry discipline (ADR-0072/0079), the two-step factory (ADR-0079), the iteration-status protocol (ADR-0038/0213), atomic landing + six-gate (ADR-0052), byte-stable PARSE-REJECT wording (ADR-0080).
- **NOT consumed:** the ADR-0219 upstream-cluster-override seam (mongo never overrides routing); the zookeeper latency-threshold counter machinery (mongo has no fast/slow counters — its latency signal is histogram-only and therefore deferred).

---

## 3. Sub-phase scope summary

### 3.0 Split disposition — PRE-CONFIRMED at BRAINSTORM Q1

The 3-way FEATURE-PROGRESSIVE pre-split was settled at BRAINSTORM Q1 (ROADMAP rows 29.1/29.2/29.3 already `planned`). No SPEC-time re-decision. The D29-11 envelope (§11.11 + §15) confirms each sub-phase fits the ADR-0045 gate individually; 29.1 is the largest (~14-19 tasks) but does not straddle the gate the way 28.1 did (mongo's fixed roster is 23 stats, not 201). Each sub-phase re-checks at its own PLAN.

### 3.1 Split surface-mapping table (per phase-22/25/26/28 §3.1 precedent)

| Surface element | 29.1 | 29.2 | 29.3 |
|---|---|---|---|
| `internal/filter/network/mongoproxy/` package + TypeURL + 5-field config parse | **lands** | — | — |
| In-house BSON parser (`bson.go`; the 14-type upstream subset) | **lands** | — | — |
| Wire decoder: MsgHeader + OP_QUERY/OP_INSERT/OP_GET_MORE/OP_KILL_CURSORS/OP_COMMAND | **lands** | — | — |
| Wire decoder: OP_REPLY/OP_COMMANDREPLY (in `OnWrite`) | — | **lands** | — |
| The 23-stat fixed roster creation under `mongo.<stat_prefix>.` (creation parity) | **lands (full roster)** | — | — |
| Request-side fixed increments (13: `op_query` + 7 `op_query_*` + `op_insert`/`op_get_more`/`op_kill_cursors`/`op_command` + `decoding_error`) | **lands** | — | — |
| Response-side fixed increments (7: `op_reply` + 3 reply-flag + `op_command_reply` + 2 `cx_destroy_*`) | — | **lands** | — |
| `delays_injected` + `cx_drain_close` increments | — | — | **lands** |
| Dynamic `cmd.<cmd>.total` + `collection.<c>.query.*` + callsite counter families | **lands** | — | — |
| The `op_query_active` GAUGE (inc at query decode; dec at correlated reply / destroy) | created 29.1 | **inc/dec lands** | — |
| Per-connection active-query list (requestID + collection + start time) | **lands (written)** | consumed | — |
| requestID↔responseTo correlation + per-connection mutex (ADR-0223 pattern) | — | **lands** | — |
| Dynamic-metadata emission (`emit_dynamic_metadata` → ADR-0217 Bucket) | — | **lands** | — |
| The async halt/resume seam (active async ContinueReading + cross-goroutine safety + post-handoff honoring) | — | — | **lands** |
| Fault-delay injection (`delay` FaultDelay; deterministic 100%-probability differential arms) | parsed | — | **lands** |
| Mongo access log (`access_log` path; JSON formatter; AsyncFileSink) | parsed | — | **lands** |
| Close-direction seam OR coverage boundary (D-P4, AMEND-B12) | — | **lands** | — |
| 8th `builtins.RegisterBuiltins` registration + `bootstrap.go` blank-import | **lands** | — | — |
| `internal/stats/name.go` `mongo.` TAG-EXTRACTOR arm (ADR-0218 shape, AMEND-B2) | **lands** | — | — |
| Differential fixtures | +2 (`0049-mongo-requests` cross-side, `0050-mongo-boot-reject`) | +1 (`0051-mongo-responses` cross-side) | +1 (`0052-mongo-fault-delay` cross-side) |
| New fuzzers | +1 (wire+BSON decode, 39th) | extend-or-add (D-P6; anticipated extend → stays 39) | — |
| BEHAVIOR_CONTRACT bundle | 29.1 bundle | 29.2 bundle (+ histogram coverage boundary) | 29.3 bundle (+ access-log + runtime-key boundaries) |
| Anticipated ADRs | 0224 | 0225 | 0226 |
| ROADMAP | 29.1 `planned → in-progress` at 29.1 SPEC; `→ done` at 29.1 IMPL | 29.2 same | 29.3 same; **parent row 29 ROLLUP `→ done` ATOMICALLY with 29.3** |

### 3.2 Per-sub-phase scope detail

**29.1 `network-filter-mongo-wire-and-requests`** — (a) the **`internal/filter/network/mongoproxy/` package, request side**: TypeURL via `proto.MessageName` (§5.1), config parse of the 5-field proto (`stat_prefix` PGV-required → boot-reject; `delay` parsed + PGV-validated here [AMEND-B9 arms] but consumed at 29.3; `access_log` path parsed here, consumed at 29.3; `emit_dynamic_metadata` parsed here, consumed at 29.2; `commands` list with the default `{delete, insert, update}` + the remembering semantics per AMEND-B7), the **in-house BSON parser** (`bson.go` — little-endian; the 14-type upstream subset with throw-on-unknown-type, §11.4 item 5; document/element walk; full eager parse mirroring upstream), the **wire decoder request side** (16-byte LE MsgHeader; the 7-opcode dispatch with `decoding_error` on unknown opcodes incl. OP_MSG; OP_QUERY/OP_INSERT/OP_GET_MORE/OP_KILL_CURSORS/OP_COMMAND body decode per §11.4 item 4; decoder-internal reassembly via the private-buffer model per AMEND-B6/§11.5; unconditional passthrough; sniffing-off-on-error), request-side fixed counters (13 of the 23), the dynamic `cmd.*`/`collection.*`/callsite counter families (counters only — `reply_*` histograms deferred), the **per-connection active-query list** (requestID + collection + command + start time; written at 29.1, consumed at 29.2) + the `op_query_active` gauge CREATED (increments live at 29.2); (b) registration as the **8th built-in** + the `mongo_proxy/v3` blank-import in `bootstrap.go`; (c) the **`mongo.` TAG-EXTRACTOR arm** in `internal/stats/name.go` (AMEND-B2 — `mongo.<prefix>.<rest>` → name `envoy_mongo_<rest flattened>`, label `envoy_mongo_prefix="<prefix>"`); (d) fixtures **`0049-mongo-requests`** (cross-side; §8.1) + **`0050-mongo-boot-reject`** (§8.2); (e) the **wire+BSON decode fuzzer** (39th); (f) ADR-0224 §Decision/§Consequences body; BEHAVIOR_CONTRACT 29.1 bundle; STATE/ROADMAP advance (sub-row 29.1 `→ done`).

**29.2 `network-filter-mongo-responses-and-correlation`** — (a) the **response-side decoder** in `OnWrite` (the ADR-0221 write chain; consumer #2): OP_REPLY decode (responseFlags + cursorID + startingFrom + numberReturned + documents) + OP_COMMANDREPLY decode; response-side fixed counters (`op_reply`, `op_reply_cursor_not_found` [flag 0x01], `op_reply_query_failure` [flag 0x02], `op_reply_valid_cursor` [cursorID ≠ 0], `op_command_reply`); (b) **requestID↔responseTo correlation** consuming the 29.1 active-query list (first-match-erase per §11.4 item 7) + the **`op_query_active` gauge increments** (inc at query decode [29.1's list-append site gains the inc], dec at correlated reply + at connection destroy) + the per-connection mutex (the ADR-0223 pattern — OnWrite on pump B reads/erases the list OnData on pump A writes); (c) `cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq` + the close-direction seam or boundary (D-P4/AMEND-B12); (d) **dynamic-metadata emission** (AMEND-B11 shape onto the ADR-0217 Bucket; unit-test proof, D-P11); (e) fixture **`0051-mongo-responses`** (cross-side; §8.3 — incl. the gauge quiesced-point arms); (f) the fuzzer extend-or-add decision (D-P6); (g) ADR-0225 body; BEHAVIOR_CONTRACT 29.2 bundle (+ the histogram coverage boundary).

**29.3 `network-filter-mongo-fault-delay-and-access-log`** — (a) the **async halt/resume seam extension** (§4.1; ADR-0226): active asynchronous `ContinueReading` + cross-goroutine safety (a mutex/atomic design on `chainRuntime` halt state — exact shape at the 29.3 SPEC, D-P12) + post-handoff read-halt honoring in `replayRead`/`readChainConn.Read`; never-halting chains byte-identical (R1-style equivalence); zero new exported API anticipated; (b) **fault-delay injection**: the parsed `delay` config consumed — per-decoded-request-message evaluation with re-entrancy guard (AMEND-B8), `rollPercent`-style percentage evaluation (the phase-09 precedent), `time.AfterFunc` timer → `ContinueReading()` on fire, `delays_injected` at arm time, StopIteration-while-pending from `OnData`/replay, timer cancel on connection destroy; (c) the **mongo access log**: the `access_log` path consumed — the AMEND-B10 JSON line format (`time`/`message`/`upstream_host`), a formatter seam or mongo-owned sink over `internal/accesslog` (D-P7), unit-test proof + coverage boundary (NO fixture dir); (d) **`cx_drain_close`**: posture vs the phase-08.2 drain machinery (increment on reply-completion when the drain decision says close + the list is empty; close type FlushWrite per §11.10 — the drain-integration shape is a 29.3 SPEC refinement); (e) fixture **`0052-mongo-fault-delay`** (cross-side; §8.4 — DETERMINISTIC 100%-probability arms; `delays_injected` parity; timing never compared); (f) ADR-0226 body; BEHAVIOR_CONTRACT 29.3 bundle (+ the access-log + runtime-key boundaries); the **parent-row-29 ROLLUP** (parent flips `in-progress → done` ATOMICALLY with sub-row 29.3 per the 18/19/22/24/25/26/28 precedent) + the six-gate.

---

## 4. Framework primitives — 1 framework-seam EXTENSION (29.3) + 1 NEW filter package (29.1/29.2/29.3)

### 4.1 EXTENSION: the async halt/resume seam (ADR-0226; lands at 29.3)

In the existing `internal/filter/network/` package (NOT a new package). The as-built anchor points this extends (verified this session against master `386e6dd`):

- `internal/filter/network/chain.go:341-374` — `runData()`: the dispatch loop whose per-pass-stop semantics (OnData StopIteration → `resumeIdx` parked at the filter; next read re-dispatches it; `connHalted` NOT set) ALREADY mirror upstream's repeated-StopIteration model pre-handoff (AMEND-B13).
- `internal/filter/network/chain.go:439-450` — `callbacks.ContinueReading()`: two as-built paths (parked OnNewConnection halt → synchronous resume; re-entrant in-OnData → `resumeRequested` flag). The asynchronous third context (called from a timer goroutine while no dispatch is running) currently degenerates to a flag-set nothing consumes — THE GAP.
- `internal/filter/network/chain.go:389-395` — `replayRead()`: the post-handoff observational pass that ignores Status and drains unconditionally — THE SECOND GAP (the 28.1b SPEC §3.5 boundary, lifted here for halt purposes only).
- `internal/filter/network/readconn.go:34-43` — `readChainConn.Read`: returns socket bytes to the terminal's pump regardless of filter Status — THE THIRD GAP.
- `internal/filter/network/chain.go:144-145, 230-232, 246-285` — the single-goroutine pre-handoff model, `terminalReady()`, and `handleTerminal`'s wrap composition (`writeChainConn(prefixConn(readChainConn(rawConn)))`).
- `internal/filter/network/zookeeperproxy/decoder.go:62-70` — the ADR-0223 per-connection-mutex precedent (lock only the cross-goroutine state; single-owner state stays lock-free).

The seam's pinned shape (three extensions; design detail at the 29.3 SPEC):

1. **Active asynchronous `ContinueReading`.** A `ContinueReading()` call arriving when (a) the chain is NOT dispatching and (b) `connHalted` is false but `resumeIdx` is parked at a filter that returned StopIteration, must ACTIVELY resume: advance `resumeIdx` past the halting filter, dispatch the buffered bytes to the remaining chain (upstream `std::next(filter->entry())` parity — the halting filter is NOT re-invoked with the same bytes), and re-evaluate terminal readiness (so a halt that deferred handoff hands off after resume). Upstream semantics per §11.7.
2. **Cross-goroutine safety.** The resume call originates on a `time.AfterFunc` goroutine; it races (pre-handoff) the listener read-loop goroutine calling `onData`/`runData`, or (post-handoff) pump A calling `replayRead`. The synchronization design (anticipated: a `chainRuntime` mutex guarding the halt/resume state + buffer, scoped per the ADR-0223 minimal-critical-section discipline) is pinned at the 29.3 SPEC (D-P12). Never-halting chains must not pay measurable synchronization cost on the hot path (the equivalence argument R1 covers semantics; the 29.3 SPEC addresses overhead).
3. **Post-handoff read-halt honoring.** While a filter has halted the chain (returned StopIteration from a replayed `OnData` and not yet resumed): `replayRead` must stop dispatching at the halting filter, NOT drain the held bytes, and `readChainConn.Read` must NOT return those bytes to the terminal's pump (the pump blocks or the bytes are withheld until resume — mechanism at the 29.3 SPEC). `ContinueReading` releases: the held bytes flow to the remaining filters and then to the pump. For never-halting filters (zookeeper, and mongo when no delay is configured) the path is byte-identical to today — the 28.1b §3.5 pure-observation semantics are unchanged for them.

`TerminalFilter.Handle` signature UNCHANGED; `tcp_proxy`/HCM untouched; zero-write-filter chains and never-halting chains byte-identical (R1). Anticipated zero new exported API: the extension deepens the semantics of `ContinueReading()` + `replayRead` + `readChainConn.Read`, none of which are exported-surface changes.

### 4.2 NEW: `internal/filter/network/mongoproxy/` (ADR-0224 + ADR-0225; lands across 29.1/29.2/29.3)

Go package `mongoproxy` (single-token-joined per the `directresponse`/`snicluster`/`zookeeperproxy` precedent). Implements BOTH `ReadFilter` and `WriteFilter` (one instance per connection). Anticipated layout (29.x SPECs/PLANs finalize the file split): `mongoproxy.go` (TypeURL + `NewFactory`), `config.go` (5-field parse + FaultDelay validation + the commands set), `bson.go` (the in-house BSON parser), `codec.go` (MsgHeader framing + per-opcode message decode), `stats.go` (the 23-stat roster + dynamic-name helpers), `filter.go` (the ReadFilter/WriteFilter glue + active-query list + fault delay + access log + metadata). The BSON parser stays INSIDE the package (extract-at-second-consumer; YAGNI).

### 4.3 Framework-delta accretion shape

Phase 29 continues framework GROWTH: the async halt/resume seam is the framework's FIFTH structural extension and the SECOND deferred-with-allowance surface consumed by the consumer it was anticipated for (ADR-0221 §Consequences "mongo fault-delay is the anticipated first halt consumer" → ADR-0226) — the defer-with-allowance / consume-at-consumer discipline working as designed, twice in a row.

---

## 5. Proto-field roster (per §11.3 D29-2)

All rosters transcribed from go-control-plane v1.32.4 (`extensions/filters/network/mongo_proxy/v3/mongo_proxy.pb.go` + `.pb.validate.go`; `extensions/filters/common/fault/v3/fault.pb.go` + `.pb.validate.go`); verified by `proto.MessageName` run in-session.

### 5.1 TypeURLs

- `proto.MessageName(&mongo_proxyv3.MongoProxy{})` = `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` → **`@type` = `type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy`** (the `extensions.` segment per `reference_network_filter_typeurl_extensions`; pinned by an IMPL Task-1 `proto.MessageName` test, never the docs string).
- `proto.MessageName(&faultv3.FaultDelay{})` = `envoy.extensions.filters.common.fault.v3.FaultDelay` (embedded message; no `@type` of its own in bootstrap YAML).

### 5.2 `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` (5 fields; `[#next-free-field: 6]`)

| Go field | proto field | tag | Go type | default / PGV | 29.x disposition |
|---|---|---|---|---|---|
| `StatPrefix` | `stat_prefix` | 1 | `string` | **PGV-required min 1 rune** | REQUIRED → boot-reject (29.1; the `0050` fixture) |
| `AccessLog` | `access_log` | 2 | `string` | no PGV; empty = no access log | parsed 29.1; consumed 29.3 (file path for the JSON access log; runtime-gated upstream — gating is a departure) |
| `Delay` | `delay` | 3 | `*faultv3.FaultDelay` | nil = no fault injection; embedded PGV recurse | parsed + PGV-validated 29.1 (AMEND-B9 arms); consumed 29.3 |
| `EmitDynamicMetadata` | `emit_dynamic_metadata` | 4 | `bool` | false | parsed 29.1; consumed 29.2 (gates the AMEND-B11 Bucket writes) |
| `Commands` | `commands` | 5 | `[]string` | **empty = `{"delete", "insert", "update"}`** (AMEND-B7) | SUPPORT (29.1; the builtin-remembering set for `cmd.*` naming) |

Upstream doc notes: `delay` — "Delays are applied to the following MongoDB operations: Query, Insert, GetMore, and KillCursors. Once an active delay is in progress, all incoming data up until the timer event fires will be a part of the delay." `commands` — "Defaults to 'delete', 'insert', and 'update'. … metrics will not be emitted for 'find' commands, since those are considered queries" (the find→query alias path, AMEND-B7). No `[#not-implemented-hide:]` markers on any field (unlike zookeeper's `access_log` — mongo's access log IS implemented upstream).

### 5.3 `envoy.extensions.filters.common.fault.v3.FaultDelay` (oneof + percentage) + `envoy.type.v3.FractionalPercent`

| Go field / oneof arm | proto field | tag | Go type | PGV | 29.x disposition |
|---|---|---|---|---|---|
| oneof `fault_delay_secifier` (sic — upstream typo) | — | — | — | **REQUIRED** ("value is required") | absent → boot-reject (AMEND-B9) |
| `FaultDelay_FixedDelay.FixedDelay` | `fixed_delay` | 3 | `*durationpb.Duration` | **gt 0s** | the only arm mongo uses; ≤ 0 → boot-reject |
| `FaultDelay_HeaderDelay_.HeaderDelay` | `header_delay` | 5 | empty message | none | HTTP-only variant; if configured for mongo: parse-accept (no delay results — upstream's `FixedDelayProvider` path is never taken); 29.3 SPEC refines (D-P5) |
| `Percentage` | `percentage` | 4 | `*typev3.FractionalPercent` | optional; recurse | numerator/denominator evaluation per the phase-09 `percentageToFloat` precedent |

`FractionalPercent`: `numerator` uint32 (default 0) / `denominator` enum `HUNDRED=0` / `TEN_THOUSAND=1` / `MILLION=2`. FaultDelay proto tags 1 and 2 are reserved (legacy). NOTE: go-control-plane v1.32.4 ships no generated `Validate()` for FractionalPercent — the `defined_only` denominator rule is descriptor-only; envoy-go's parse treats an out-of-range denominator as a reject for parity (29.1 SPEC pins the exact arm).

---

## 6. PARSE-REJECT roster (per §11.3 + AMEND-B9)

### 6.1 Wording discipline

Per ADR-0080 byte-stable PARSE-REJECT discipline: each arm is a named constant with byte-stable wording verified by a table test at IMPL. Boot-reject PARITY arms (mirroring an upstream PGV/config-load failure) are distinguished from envoy-go-strict DEPARTURE arms. Phase 29 has NO departure-class rejects — every reject below mirrors an upstream PGV reject; the departures in this phase are all behavioral (runtime keys at defaults; histogram deferral; access-log gating), recorded in BEHAVIOR_CONTRACT, never as rejects.

### 6.2 29.1 PARSE-REJECT arms (all parse code lands at 29.1 since 29.1 parses the full 5-field proto)

- `mongo-stat-prefix-required` — boot-reject PARITY (the `stat_prefix` PGV min-1-rune rule, §5.2). The load-bearing `0050` fixture arm.
- `mongo-delay-specifier-required` — boot-reject PARITY (the FaultDelay oneof PGV `required` rule — `delay: {}` rejects, AMEND-B9).
- `mongo-delay-fixed-delay-too-small` — boot-reject PARITY (the `fixed_delay` PGV `gt 0s` rule — `fixed_delay: 0s` rejects, AMEND-B9).
- Framework-level: unknown network-filter `typed_config` type_url → existing boot-reject (no new arm).
- `access_log`: NOT a reject (any string accepted; empty = disabled — upstream parity).
- `commands`: NOT a reject (any list accepted incl. empty-means-default — upstream parity).

Whether the two delay arms gain `0050` FIXTURE arms at 29.1 or stay unit-test-only until 29.3 (when `delay` is consumed) is D-P5 (anticipated: unit-test-only at 29.1; the `0050` fixture carries the `stat_prefix` arm only — the zookeeper D-P4 precedent).

---

## 7. Stat surface (per §11.1 D29-1 + §11.2/§11.3 D29-3 + AMEND-B1/B2/B3/B4)

### 7.1 Scope/prefix shape — `mongo.<stat_prefix>.<counter>` (AMEND-B1)

Upstream: `config.cc:24` `fmt::format("mongo.{}", proto_config.stat_prefix())`; both the fixed macro stats (`POOL_*_PREFIX`) and the dynamic `MongoStats` families prepend this same string. Emitted names: `mongo.<stat_prefix>.<counter>` (confirmed live: `mongo.mongoprobe.op_query`, `mongo.otherprefix.cx_drain_close`). envoy-go mirrors this internal naming exactly (the differential `StatsAsserter` + the Prometheus arm depend on it).

### 7.2 The 23-stat fixed roster (AMEND-B3/B4) — exact transcription of `ALL_MONGO_PROXY_STATS`

**22 COUNTERS:** `cx_destroy_local_with_active_rq`, `cx_destroy_remote_with_active_rq`, `cx_drain_close`, `decoding_error`, `delays_injected`, `op_command`, `op_command_reply`, `op_get_more`, `op_insert`, `op_kill_cursors`, `op_query`, `op_query_await_data`, `op_query_exhaust`, `op_query_multi_get`, `op_query_no_cursor_timeout`, `op_query_no_max_time`, `op_query_scatter_get`, `op_query_tailable_cursor`, `op_reply`, `op_reply_cursor_not_found`, `op_reply_query_failure`, `op_reply_valid_cursor`.

**1 GAUGE:** `op_query_active` (Accumulate — inc at `ActiveQuery` construction, dec at destruction).

**0 HISTOGRAMS** in the fixed macro (AMEND-B3 — the `HISTOGRAM` macro argument is declared but unused; all mongo histograms are dynamic-family members, §7.3).

| Family | Count | Created | Incremented | Notes |
|---|---|---|---|---|
| Request-side ops: `op_query` + 7 `op_query_*` + `op_insert`/`op_get_more`/`op_kill_cursors`/`op_command` + `decoding_error` | 13 | 29.1 | 29.1 | flag counters per §11.4 item 4 bit values; `op_query_no_max_time` only for non-command queries with maxTime < 1; `decoding_error` at most once per connection (AMEND-B6) |
| Response-side ops: `op_reply` + `op_reply_cursor_not_found`/`op_reply_query_failure`/`op_reply_valid_cursor` + `op_command_reply` | 5 | 29.1 | 29.2 | reply flags 0x01/0x02; valid_cursor = cursorID ≠ 0 |
| Connection lifecycle: `cx_destroy_local_with_active_rq`/`cx_destroy_remote_with_active_rq` | 2 | 29.1 | 29.2 | needs close direction (AMEND-B12/D-P4) |
| Fault + drain: `delays_injected`, `cx_drain_close` | 2 | 29.1 | 29.3 | `delays_injected` at timer-arm time |
| Gauge: `op_query_active` | 1 | 29.1 | 29.2 | the project's first mirrored gauge |
| **Total** | **23** | | | |

**Creation timing (AMEND-B4 + D-P1):** upstream creates the fixed roster per-ProxyFilter-instance (= per connection; pool-deduped), so the reference admin shows NOTHING until the first downstream connection. envoy-go's anticipated posture (D-P1, resolved at the 29.1 SPEC): EAGER creation at config parse (freeze-after-boot friendly, the zookeeper precedent), with the boot-window visibility difference (envoy-go exposes `mongo.*` at 0 from boot; reference exposes nothing until first connection) recorded as a BEHAVIOR_CONTRACT departure that is UNOBSERVABLE to the differential because every fixture assertion runs post-first-connection (the §8 fixture discipline).

**Dynamic (non-macro) counter families (29.1; the zookeeper `auth.<scheme>_rq` convention — NOT counted in the static 360):**

- `mongo.<sp>.cmd.<cmd>.total` — per command-name; `<cmd>` is the builtin-remembered name or `unknown_command` (AMEND-B7).
- `mongo.<sp>.collection.<collection>.query.total` (always) / `.query.scatter_get` (no `_id` in the query doc) / `.query.multi_get` (`_id` is a Document/Array) — per collection; PrimaryKey-type queries emit only `.total`.
- `mongo.<sp>.collection.<collection>.callsite.<callsite>.query.total`/`.scatter_get`/`.multi_get` — only when a `$comment` JSON callsite is present (counters only; INCLUDED at 29.1 per the Q1 full-counter-parity envelope — the callsite `reply_*` histograms are deferred with the rest).

**Dynamic HISTOGRAM families (ALL deferred per ADR-0060; coverage boundary at 29.2):** `cmd.<cmd>.reply_num_docs`/`.reply_size`/`.reply_time_ms`; `collection.<c>.query.reply_*`; `collection.<c>.callsite.<cs>.query.reply_*`.

### 7.3 Project stat-count delta

337 → **360** at family-row-done (+23; all created at 29.1 creation-parity, increments staged across 29.1/29.2/29.3). Dynamic cmd/collection/callsite counters excluded from the static count (config/traffic-dependent, lazily created — the rbac `policy.<name>.*` / zookeeper `auth.<scheme>_rq` precedent).

### 7.4 Prometheus exposition — the `mongo.` TAG-EXTRACTOR arm (AMEND-B2)

Reference Envoy v1.37.2 `/stats/prometheus` (probed live, two stat_prefix values): mongo stats emit as **`envoy_mongo_<leaf>{envoy_mongo_prefix="<stat_prefix>"} <v>`** — the metric name is flat (`envoy_mongo_op_query`), the stat_prefix is hoisted into the `envoy_mongo_prefix` label, every metric family has a `# TYPE` line, and the gauge declares `gauge` type. This is upstream tag extraction (the `envoy.mongo_prefix` well-known tag).

envoy-go's `internal/stats/name.go` default branch errors on unrecognized prefixes, so a new arm is required — and it is the **`.rbac.` TAG-EXTRACTOR shape (ADR-0218)**, NOT the ADR-0138 inline-prefix shape the BRAINSTORM/ROADMAP hypothesized: detect `mongo.<prefix>.<rest>` (leading literal `mongo.` + a dot-free `<prefix>` segment) → metric name `envoy_mongo_` + `<rest>` flattened (dot→underscore) + label `envoy_mongo_prefix="<prefix>"`. The fixed-roster shape is pinned by this probe; the DYNAMIC-name shape (`cmd.*`/`collection.*` — does the label hoist apply identically, leaving e.g. `envoy_mongo_cmd_isMaster_total{envoy_mongo_prefix="…"}`?) could not be live-probed this session (§11.1 caveat) and is re-pinned at the 29.1 SPEC (D-P2).

### 7.5 envoy-go-strict / envoy-go-lenient departure flags (BEHAVIOR_CONTRACT at each IMPL)

- The dynamic HISTOGRAM families unmirrored (ADR-0060; coverage boundary at 29.2).
- Runtime-key gating unmirrored — the filter behaves at key defaults: sniffing always on, logging always on (when a path is configured), drain-close always on, fault percentage/duration from the proto (§2.1; coverage boundary at 29.3).
- The boot-window stat-creation difference (D-P1; unobservable to the differential).
- Access-log differential comparison not viable (AMEND-B10; unit-test + coverage boundary at 29.3).
- Dynamic-metadata emission differential-invisible (AMEND-B11; unit-test + coverage note at 29.2).
- Close-direction availability (AMEND-B12; either a small seam at 29.2 or a coverage boundary — D-P4).

---

## 8. Differential fixture taxonomy (+4 across three sub-phases)

Full cross-side against reference Envoy v1.37.2. Per `reference_differential_fixture_dispatch_constraint`: cross-side and boot-reject fixtures are SEPARATE directories. Per `reference_differential_asserter_dispatch`: every subject-side stat assertion uses `fixture.StatsAsserter` and MUST be proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`). The body differential is intrinsically vacuous (bytes pass through unchanged on both sides) — the stat comparison IS the proof. Numbering continues from `0048` (master tip tail); re-pinned at each sub-phase IMPL Task 1.

**Fixture-design caveats from the §11.1 live probe:** (i) the fixture backend MUST be a real listening TCP responder — if tcp_proxy cannot establish its upstream connection, the reference closes the downstream connection before the mongo decoder ever runs (zero decode, zero stats); (ii) all stat assertions are post-first-connection (AMEND-B4); (iii) after a `decoding_error` fires, that connection decodes NOTHING further (AMEND-B6) — arms needing further decode use fresh connections.

### 8.1 `0049-mongo-requests` (29.1; cross-side)

Chain `[mongo_proxy, tcp_proxy]` on BOTH sides; the fixture backend is a canned-bytes TCP responder (accepts, reads, need not reply for request-side arms — but MUST be listening). Driver sends hand-crafted little-endian wire bytes (§11.4 layouts). Arms (29.1 SPEC finalizes):

1. **plain query** — OP_QUERY to `db.collection1` with a `{a: 1}` BSON query (no `_id`) → `op_query` +1, `op_query_scatter_get` +1, `collection.collection1.query.total` +1, `collection.collection1.query.scatter_get` +1, `op_query_no_max_time` +1 both sides.
2. **$cmd command** — OP_QUERY to `admin.$cmd` with `{isMaster: 1}` + config `commands: [isMaster]` → `op_query` +1, `cmd.isMaster.total` +1; a second command NOT in the list → `cmd.unknown_command.total` +1 (the AMEND-B7 remembering-semantics proof).
3. **query-shape variants** — a `{_id: <scalar>}` query (PrimaryKey → only `.query.total`); a `{_id: {$in: […]}}`-style document `_id` (MultiGet → `.query.multi_get` + `op_query_multi_get`); flag-bit arms (tailable_cursor 0x02 / no_cursor_timeout 0x10 / await_data 0x20 / exhaust 0x40 → their `op_query_*` counters).
4. **other request opcodes** — OP_INSERT (+ `op_insert`), OP_GET_MORE (+ `op_get_more`), OP_KILL_CURSORS (+ `op_kill_cursors`), OP_COMMAND (+ `op_command`) → each +1 both sides.
5. **$comment callsite** — a query with `$comment: "{\"callingFunction\": \"fixtureFn\"}"` → `collection.<c>.callsite.fixtureFn.query.total` +1 (the callsite-family inclusion proof).
6. **unsupported-opcode arm (fresh connection)** — an OP_MSG (2013) frame → `decoding_error` +1 both sides; passthrough proven byte-exact through tcp_proxy; NO further decode on this connection (AMEND-B6).
7. **garbage-BSON arm (fresh connection)** — a well-framed OP_QUERY whose BSON document is malformed → `decoding_error` +1 both sides.
8. **deliberate-break liveness proof** — recorded in driver comments + README per the `0030` lesson, run with `-count=1`.

### 8.2 `0050-mongo-boot-reject` (29.1; boot-reject)

Missing `stat_prefix` → both sides reject at boot (PGV-mirror; boot-stderr substring parity). The AMEND-B9 delay arms (`delay: {}`; `fixed_delay: 0s`) are unit-tested at 29.1; whether they gain fixture arms here is D-P5.

### 8.3 `0051-mongo-responses` (29.2; cross-side)

Chain `[mongo_proxy, tcp_proxy]` both sides; the backend replies to each request with hand-crafted OP_REPLY bytes (responseTo = the request's requestID). Arms (29.2 SPEC finalizes): query→reply round trips → `op_reply` + `op_reply_valid_cursor` (cursorID ≠ 0 arm) / `op_reply_cursor_not_found` (flag 0x01 arm) / `op_reply_query_failure` (flag 0x02 arm) parity; OP_COMMAND→OP_COMMANDREPLY → `op_command_reply`; the **`op_query_active` GAUGE** asserted at quiesced points (all queries answered → 0 both sides; an unanswered-query arm → 1 both sides — the project's first gauge differential, D-P9/D-P10); `cx_destroy_remote_with_active_rq` (the driver closes the connection with a query outstanding — both sides see remote close).

### 8.4 `0052-mongo-fault-delay` (29.3; cross-side)

Chain `[mongo_proxy, tcp_proxy]` both sides with `delay: {fixed_delay: 0.100s, percentage: {numerator: 100, denominator: HUNDRED}}` (100% probability → DETERMINISTIC). Arms: a query through the delayed filter → `delays_injected` +1 both sides + the traffic still completes (passthrough-not-broken; the reply round-trips after the delay); a no-delay listener arm (no `delay` config) → `delays_injected` stays 0 + byte-identical behavior (the seam's non-perturbation proof); timing itself NEVER compared (BOOTSTRAP §7.2). NO access-log dir (AMEND-B10).

### 8.5 Total fixture-dir count

50 → **54** (+2 at 29.1; +1 at 29.2; +1 at 29.3). No new conformance harness (matches 26/27/28); h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at each sub-phase six-gate.

---

## 9. Behavior-contract delta (per ADR-0052 atomic landing)

BEHAVIOR_CONTRACT.md gains phase-29 content in three passes (one bundle per sub-phase IMPL final task):

- **29.1 bundle**: NEW `### envoy.filters.network.mongo_proxy` subsection (request-side semantics; the 23-stat roster + creation posture per D-P1; the `mongo.<stat_prefix>.` scope; the Prometheus tag-extractor flattening; the 7-opcode decode envelope + OP_MSG-not-decoded; the sniffing-off-on-error semantics; the dynamic cmd/collection/callsite counter families + the commands-remembering semantics; the runtime-keys-at-defaults departure); stat table 337 → 360.
- **29.2 bundle**: the response-side + correlation + gauge extension of the mongo subsection; the dynamic-HISTOGRAM coverage-boundary record (ADR-0060); the dynamic-metadata emission note (differential-invisible); the close-direction resolution (seam or boundary per D-P4).
- **29.3 bundle**: the fault-delay + access-log + drain extension; the access-log coverage boundary (timing-bearing format); the async halt/resume seam semantics (`### Network filter chain framework — async halt/resume (29.3 amendment)` block); parent-row-29 family rollup note.

---

## 10. ADR anchor map (3 §Context drafts at THIS parent-SPEC commit)

Per ADR-0044 (§Context at SPEC; §Decision/§Consequences at IMPL) + the BRAINSTORM §7 locked numbering + the phase-25/26/28 parent-SPEC precedent. At THIS parent-SPEC commit, ADR-0224 + ADR-0225 + ADR-0226 §Context drafts are appended to DECISIONS.md (tail ADR-0223 → ADR-0226; next-free → ADR-0227). The 29.1 IMPL lands the 0224 body; the 29.2 IMPL lands the 0225 body; the 29.3 IMPL lands the 0226 body. (Per the phase-28 precedent of drafting all sub-phase ADRs at the parent SPEC: all three §Contexts are fully determined by THIS session's empirical pins; sub-phase SPECs may amend in place.)

- **ADR-0224** *(29.1)* — the `mongo_proxy` filter, request side: 5-field config parse (+ FaultDelay PGV arms), the in-house BSON parser (the 14-type upstream subset), the little-endian wire decoder (the 7-opcode envelope; OP_MSG not decoded), the 23-stat fixed roster under `mongo.<stat_prefix>.`, request-side increments, the dynamic cmd/collection/callsite counter families, the active-query list, the 8th built-in, the `mongo.` Prometheus TAG-EXTRACTOR arm, fixtures `0049`/`0050`, the 39th fuzzer.
- **ADR-0225** *(29.2)* — the response side: OP_REPLY/OP_COMMANDREPLY decode in `OnWrite`, requestID↔responseTo correlation + the per-connection mutex, the `op_query_active` gauge (the project's first mirrored gauge), `cx_destroy_*` counters + the close-direction resolution, dynamic-metadata emission (ADR-0217 Bucket third production write), fixture `0051`.
- **ADR-0226** *(29.3)* — the async halt/resume seam extension (active async ContinueReading + cross-goroutine safety + post-handoff read-halt honoring) + fault-delay injection + the mongo access log + `cx_drain_close` + fixture `0052` + the parent-29 ROLLUP; CONSUMES the ADR-0221 §Consequences anticipated-halt-consumer allowance.

Next-free after phase-29 phase-done ≈ **ADR-0227**.

---

## 11. Empirical-pin block (D29-1..D29-12 resolved at this SPEC session)

Parallel-subagent-fan-out scrape executed during this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-03.** The 12 pins span all three sub-phases; resolved once here; sub-phase SPECs reference this block.

**Reference source corpus:**

1. **The live `envoyproxy/envoy:v1.37.2` docker image** (image id `c5e8a68e52f4`, present locally): `--mode validate` + a real boot of a `[mongo_proxy, tcp_proxy]` listener + admin `/stats` and `/stats/prometheus` scrapes (two stat_prefix values: `mongoprobe`, `otherprefix`).
2. **go-control-plane v1.32.4 bindings** at `~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/`: `extensions/filters/network/mongo_proxy/v3/` + `extensions/filters/common/fault/v3/` + `type/v3/percent.pb.go`; `proto.MessageName` run in-session (worktree probe test, deleted after).
3. **Upstream Envoy v1.37.2 source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/filters/network/mongo_proxy/{proxy.h,proxy.cc,config.h,config.cc,mongo_stats.h,mongo_stats.cc,codec.h,codec_impl.h,codec_impl.cc,bson.h,bson_impl.h,bson_impl.cc,utility.h,utility.cc}`; `source/common/network/filter_manager_impl.{h,cc}`; `source/extensions/filters/common/fault/fault_config.{h,cc}`.
4. **envoy-go codebase** at master `386e6dd`: `internal/filter/network/{types,terminal,prefixconn,callbacks,buffer,chain,registry,readconn,writeconn}.go` + `zookeeperproxy/` + `builtins/builtins.go`; `internal/stats/{gauge,name}.go`; `internal/filter/http/fault/fault.go`; `internal/accesslog/`; `internal/dynamicmetadata/`.

### Summary disposition table (12 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D29-1 (SPEC-BLOCKING) — reference image ships mongo_proxy | **CONFIRMED** (validate OK; boots; 23-stat roster live) + REFINES creation timing + Prometheus shape | B1, B2, B3, B4 |
| §11.2 | D29-2 — proto roster + PGV + TypeURL | CONFIRMS (5 fields; stat_prefix required) + REFINES (FaultDelay oneof REQUIRED; fixed_delay gt 0s) | B9 |
| §11.3 | D29-3 — stat scope + roster + eager/lazy + commands + runtime keys | REFINES (23 fixed stats; per-connection creation; two stat systems; commands = remembering; aliasing; 6 runtime keys) | B1, B3, B4, B7 |
| §11.4 | D29-4 — wire framing + opcodes + BSON subset + OP_MSG | RESOLVES the largest unknown: **LEGACY-ONLY** (7 opcodes; OP_MSG throws); full per-opcode layouts + the 14-type BSON subset + flag bits + query-shape heuristics | B5 |
| §11.5 | D29-5 — decoding_error + passthrough + reassembly | CONFIRMS (passthrough unconditional; never closes; private-buffer copy model) + REFINES (sniffing-off-on-error for connection lifetime) | B6 |
| §11.6 | D29-6 — fault-delay semantics | REFINES (per-decoded-request-message evaluation; re-entrancy guard; delays_injected at arm; StopIteration-while-pending; timer cancel on close) | B8 |
| §11.7 | D29-7 — continueReading re-dispatch semantics | RESOLVES (resume at NEXT filter; full-chain re-dispatch per fresh read; socket reads continue during halt) | B8, B13 |
| §11.8 | D29-8 — access-log format + differential strategy | RESOLVES: timing-bearing JSON → **unit-test + coverage-boundary fallback** (no fixture dir) | B10 |
| §11.9 | D29-9 — dynamic-metadata shape + observability | RESOLVES (collection-keyed operation lists; insert/query only; cleared per decode pass; mirror at 29.2, unit-proof) | B11 |
| §11.10 | D29-10 — cx_destroy direction + drain posture | REFINES (LocalClose/RemoteClose keying — an as-built gap → D-P4; drain = reply-completion + zero-ms timer + FlushWrite) | B12 |
| §11.11 | D29-11 — per-sub-phase envelope | RESOLVES (all three fit the ADR-0045 gate; §15) | — |
| §11.12 | D29-12 — fuzzer envelope | RESOLVES (39th wire+BSON fuzzer at 29.1; 29.2 extend-or-add = D-P6, anticipated extend) | — |

### 11.1 D29-1 (SPEC-BLOCKING) — the reference image ships mongo_proxy: CONFIRMED

`docker run --rm -v …:/probe:ro envoyproxy/envoy:v1.37.2 --mode validate -c /probe/envoy.yaml` → `configuration '/probe/envoy.yaml' OK`, exit 0, for a bootstrap with a `[mongo_proxy, tcp_proxy]` filter chain (`stat_prefix: mongoprobe`; one static cluster). The boot banner's `envoy.filters.network` extension roster lists both `envoy.filters.network.mongo_proxy` and the legacy alias `envoy.mongo_proxy`. A real boot stays up with no mongo-related warnings. mongo_proxy is a CORE extension at `source/extensions/filters/network/mongo_proxy/` (not contrib). **The Q3 cross-side strategy stands.**

Admin scrapes pin: the stat scope shape (`mongo.<stat_prefix>.<counter>`, two prefixes — AMEND-B1); the 23-name roster with counter/gauge classification via `/stats/prometheus` `# TYPE` lines (AMEND-B3); per-connection creation (zero mongo stats until the first downstream connection — AMEND-B4); the Prometheus tag-extractor shape `envoy_mongo_<leaf>{envoy_mongo_prefix="<prefix>"}` (AMEND-B2).

**Probe-harness caveat (recorded honestly):** the live-traffic stretch probe (hand-crafted OP_QUERY bytes through the booted listener) did NOT produce counter increments in this session's Docker Desktop environment — the tcp_proxy upstream connection could not be established in that harness (`tcp.tcp.downstream_cx_rx_bytes_total: 0` despite established downstream connections), so the decode path never ran. Consequences: (i) the increment-path semantics in this SPEC are pinned from SOURCE (§11.3-§11.7), not from live observation; (ii) the DYNAMIC-stat live shapes (`cmd.*`/`collection.*` names + their Prometheus form) are a 29.1-SPEC re-probe obligation (D-P2) — the fixture harness (which runs real backends) is the natural vehicle; (iii) the fixture-design caveats in §8 (listening backend required; post-connection assertions; fresh connection per error arm) derive from this finding.

### 11.2 D29-2 — proto roster + PGV + TypeURLs

§5 transcribes the full roster. Key PGV facts (`mongo_proxy.pb.validate.go` + `fault.pb.validate.go`): `stat_prefix` min 1 rune (required); `access_log`/`emit_dynamic_metadata`/`commands` unconstrained; `delay` is an optional embedded message BUT when present its `fault_delay_secifier` oneof is REQUIRED ("value is required") and a `fixed_delay` arm must be `> 0s` (AMEND-B9). `proto.MessageName` (run in-session, probe test deleted after): `envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy` + `envoy.extensions.filters.common.fault.v3.FaultDelay`. The MongoProxy proto has no nested enums and no `*PerRoute` message. Upstream doc comments pin: the `commands` default `{delete, insert, update}` + the find-is-a-query exclusion; the `delay` operations list (Query/Insert/GetMore/KillCursors); the access-log runtime gating.

### 11.3 D29-3 — stat scope + roster + the two stat systems + commands + runtime keys

**Two stat systems sharing one prefix** (`mongo.<stat_prefix>` from `config.cc:24`): (1) the FIXED macro stats — `ALL_MONGO_PROXY_STATS(COUNTER, GAUGE, HISTOGRAM)` (`proxy.h:49-72`), 22 counters + 1 gauge + 0 histograms, created per-ProxyFilter-instance via `generateStats` (`proxy.cc:64` + `proxy.h:160-164` `POOL_*_PREFIX`) — per-connection but scope-pooled (AMEND-B4); (2) the DYNAMIC `MongoStats` system (`mongo_stats.cc`) — a per-filter-CONFIG shared object (`config.cc:43`) creating names lazily via symbol-table element vectors: the `cmd.*`, `collection.*.query.*`, and `collection.*.callsite.*.query.*` families (counters at request decode; `reply_*` histograms at correlated reply).

**The `commands` list** (`config.cc:37-43` + `mongo_stats.cc:29-31` + `proxy.cc:147-153`): default `{"delete", "insert", "update"}`; an explicit list replaces the default; the list pre-remembers builtin stat names — a decoded command not in the list emits `cmd.unknown_command.total` (AMEND-B7). Command names are normalized before lookup: `find` → cleared (query path via `parseFindCommand`); `collstats`→`collStats`, `dbstats`→`dbStats`, `findandmodify`→`findAndModify`, `getlasterror`→`getLastError`, `ismaster`→`isMaster` (`utility.cc:21-37`).

**Query-shape heuristics** (`utility.cc:112-140`): no `_id` field → ScatterGet (`.query.scatter_get` + `op_query_scatter_get`); `_id` of type Document/Array → MultiGet; scalar `_id` → PrimaryKey (only `.query.total`). `op_query_no_max_time` increments only for non-command queries with `maxTime < 1` (`proxy.cc:170-172`). Collection name = the substring of `fullCollectionName` after the first `.` (`utility.cc:48-55`; no dot → EnvoyException). Command detection = `fullCollectionName` contains `$cmd` (`utility.cc:75-92`); the command name = the FIRST element key of the (possibly `$query`-nested) command document.

**Runtime keys** (`proxy.h:34-42` + call sites): `mongo.proxy_enabled` (gates ALL sniffing, default 100%), `mongo.connection_logging_enabled` (per-connection log enrollment, checked once in ctor), `mongo.logging_enabled` (per-message), `mongo.drain_close_enabled` (the drain close), `mongo.fault.fixed_delay.percent`/`.duration_ms` (fault overrides; default to the proto values). envoy-go has no runtime layer → all behave at defaults (departure, §7.5).

**Reply-side dynamic stats** (`proxy.cc:220-252`): charged only after a reply correlates (`requestId() == responseTo()`); uncorrelated replies bump only the fixed `op_reply*` counters.

### 11.4 D29-4 — wire framing + the 7-opcode envelope + BSON subset (the largest unknown: RESOLVED legacy-only)

**MsgHeader** (`codec_impl.cc:344-371`): little-endian throughout (`peekLEInt`/`le32toh`); 16-byte header = messageLength(int32, INCLUDES itself) + requestID(int32) + responseTo(int32) + opCode(int32); the decoder waits for `data.length() >= message_length` before consuming (partial frames → return false, wait for more — never an error).

**The opcode enum** (`codec.h:24-35`): `Reply=1, Msg=1000, Update=2001, Insert=2002, Query=2004, GetMore=2005, Delete=2006, KillCursors=2007, Command=2010, CommandReply=2011`. **The dispatch switch** (`codec_impl.cc:372-426`) decodes EXACTLY 7: Reply, Query, GetMore, Insert, KillCursors, Command, CommandReply. Enum members Msg(1000)/Update(2001)/Delete(2006) have NO case; the modern OP_MSG(2013) is not even in the enum — all hit `default` → `EnvoyException("invalid mongo op N")` → `decoding_error` + sniffing off (AMEND-B5/B6).

**Per-opcode body layouts** (all after the 16-byte header):

| Opcode | Layout |
|---|---|
| OP_QUERY (2004) | flags(int32) → fullCollectionName(cstring) → numberToSkip(int32) → numberToReturn(int32) → query(BSON doc) → OPTIONAL returnFieldsSelector(BSON doc, iff body bytes remain) |
| OP_REPLY (1) | responseFlags(int32) → cursorID(int64) → startingFrom(int32) → numberReturned(int32) → exactly numberReturned BSON docs |
| OP_GET_MORE (2005) | ZERO(int32, discarded) → fullCollectionName(cstring) → numberToReturn(int32) → cursorID(int64) |
| OP_INSERT (2002) | flags(int32) → fullCollectionName(cstring) → 1..N BSON docs (loop to end of body) |
| OP_KILL_CURSORS (2007) | ZERO(int32, discarded) → numberOfCursorIDs(int32) → that many int64 cursorIDs |
| OP_COMMAND (2010) | database(cstring) → commandName(cstring) → metadata(BSON) → commandArgs(BSON) → 0..N inputDocs(BSON, loop) |
| OP_COMMANDREPLY (2011) | metadata(BSON) → commandReply(BSON) → 0..N outputDocs(BSON, loop) |

**Flag bits:** OP_QUERY: TailableCursor=0x02, NoCursorTimeout=0x10, AwaitData=0x20, Exhaust=0x40 (`codec.h:104-111`). OP_REPLY: CursorNotFound=0x01, QueryFailure=0x02 (`codec.h:136-141`); `op_reply_valid_cursor` = cursorID ≠ 0.

**BSON subset** (`bson_impl.cc:386-520`): a document = int32 docLength (includes itself + trailing 0x00) + elements + 0x00. The 14 handled element types: 0x01 Double, 0x02 String, 0x03 Document, 0x04 Array, 0x05 Binary, 0x07 ObjectId(12 bytes), 0x08 Boolean, 0x09 Datetime, 0x0A Null, 0x0B Regex(2 cstrings), 0x0E Symbol, 0x10 Int32, 0x11 Timestamp(int64), 0x12 Int64. ANY other type byte (incl. 0x06 Undefined, 0x0D JS code, 0x13 Decimal128) → `EnvoyException("invalid BSON element type")`. The parse is FULL/EAGER (every element materialized; nested docs recurse). BSON strings = int32 len (includes trailing NUL) + bytes; cstrings = NUL-terminated. envoy-go's `bson.go` mirrors exactly this subset + throw set.

**Correlation** (`proxy.cc:146,180, 205-252`): ONLY OP_QUERY creates `ActiveQuery` entries (GetMore/Insert/KillCursors/Command do NOT); OP_REPLY correlates by `query_info_.requestId() == message->responseTo()`, first match erased, loop breaks. Uncorrelated replies charge fixed counters only.

**$maxTimeMS / $comment** (`utility.cc:57-110`): `$maxTimeMS` (fallback `maxTimeMS`) Int32/Int64 → maxTime; `$comment` String parsed as JSON → field `"callingFunction"` → the callsite name (any failure → empty string → no callsite stats).

### 11.5 D29-5 — decoding_error + passthrough + the private-buffer copy model

`decoding_error` has exactly ONE increment site (`proxy.cc:340-345` `doDecode`): any `EnvoyException` out of `decoder_->onData(buffer)` → log + inc + **`sniffing_ = false`** — decode permanently stops for this connection; subsequent reads/writes drain the private buffers without decoding (AMEND-B6). Exception sources: unknown opcode (incl. OP_MSG), all BSON/buffer-underflow errors, `"invalid full collection name"` (no dot), `"invalid query command"` (empty $cmd doc).

**The filter is a pure copying sniffer**: `onData` does `read_buffer_.add(data)` (COPY — the chain's buffer is untouched) then `doDecode(read_buffer_)`; `onWrite` symmetric with `write_buffer_`. The decoder drains parsed messages from the PRIVATE buffers only; partial messages accumulate there across reads. The connection's own data stream is never modified, never drained by the filter, and the connection is never closed by the filter. `onWrite` ALWAYS returns Continue; `onData` returns Continue unless a delay timer pends (§11.6). envoy-go's mongoproxy mirrors this private-buffer copy model (the zookeeper `chainConsumed` high-water pattern adapts — the 29.1 SPEC pins the exact tracking shape given envoy-go's chain `Buffer` is not drained by observers).

### 11.6 D29-6 — fault-delay semantics

`tryInjectDelay()` (`proxy.cc:434-449`) is called at the top of each request-direction decode callback (decodeQuery/Insert/GetMore/KillCursors/Command/CommandReply — NOT decodeReply): re-entrancy guard (armed timer → return); `delayDuration()` (`proxy.cc:395-425`) evaluates the FractionalPercent gate (`featureEnabled(FixedDelayPercent, percentage)` — random draw against the configured percentage) then the duration (runtime override or the proto `fixed_delay`); if a delay results → create + arm a dispatcher timer → **`stats_.delays_injected_.inc()`** (at ARM time). While `delay_timer_` is non-null, `onData` returns StopIteration on every read (`proxy.cc:378-383`). Timer fire → `delayInjectionTimerCallback` (`proxy.cc:427-432`): `delay_timer_.reset()` + `read_callbacks_->continueReading()`. Connection close (either direction) while pending → timer disabled + reset (`proxy.cc:355-367`); the dtor asserts no pending timer. envoy-go 29.3 mirrors: `rollPercent`-style evaluation (phase-09 precedent), `time.AfterFunc`, inc-at-arm, StopIteration-while-pending, cancel-on-destroy.

### 11.7 D29-7 — continueReading re-dispatch semantics (shapes §4.1)

`FilterManagerImpl::onContinueReading` (`filter_manager_impl.cc:62-103`): resumes iteration at `std::next(filter->entry())` — the filter AFTER the caller; the halting filter is NOT re-invoked by the resume. The re-dispatched data is the connection's persistent read buffer (everything accumulated during the halt). Fresh socket reads during a halt: `onRead` → `onContinueReading(nullptr, …)` → iteration from the FIRST filter — so the halting filter IS re-invoked on every fresh read (and keeps returning StopIteration while its timer pends). The socket continues reading during a halt (mongo never calls readDisable); bytes accumulate in the connection read buffer. **Mapping to envoy-go (AMEND-B13):** the per-fresh-read re-dispatch = the as-built per-pass-stop (`runData` re-dispatches the parked filter on each `onData`); the resume-at-next-filter = the as-built `ContinueReading` connHalted path semantics (resumeIdx++ then runData) — what's missing is making that path fire for an ASYNC caller + post-handoff (§4.1 items 1-3).

### 11.8 D29-8 — access log: timing-bearing → unit-test + coverage-boundary fallback

`AccessLog::logMessage` (`proxy.cc:37-57`): one JSON line per decoded message, format `{"time": "<AccessLogDateTimeFormatter timestamp>", "message": <message.toString(full)>, "upstream_host": "<addr|->"}\n`, written via the AccessLogManager file API (async buffered). Request-direction messages log with `full=true` (documents dumped); replies with `full=false` (documents as counts). The `time` field is per-message wall clock → the format is timing-bearing → cross-side log comparison is non-deterministic → **the differential strategy is unit tests (format goldens against pinned inputs) + a BEHAVIOR_CONTRACT coverage boundary** (AMEND-B10). The `message.toString()` shapes per opcode (`codec_impl.cc:55-307`) are transcribed into the 29.3 SPEC for the formatter unit goldens. envoy-go side: `internal/accesslog.AsyncFileSink` is reused but its formatter is hard-wired to the HTTP `Default` (`writer.go:88`) → 29.3 adds a formatter seam or a mongo-owned sink (D-P7).

### 11.9 D29-9 — dynamic metadata: mirror at 29.2, differential-invisible

`ProxyFilter::setDynamicMetadata(operation, resource)` (`proxy.cc:77-90`): namespace `NetworkFilterNames::get().MongoProxy` (= `envoy.filters.network.mongo_proxy`); the value Struct is keyed by `resource` (the full collection name) → a ListValue of operation strings, appended per decoded message. Only `"insert"` (decodeInsert) and `"query"` (decodeQuery) are emitted in v1.37.2 (`update`/`delete` keys defined but TODO-unused — `proxy.cc:26-33`). When `emit_dynamic_metadata_` is true the namespace's fields are CLEARED at the top of every `doDecode` pass (`proxy.cc:327-334`) — the metadata reflects only the current pass. envoy-go 29.2 mirrors onto the ADR-0217 `Bucket` (`Set("envoy.filters.network.mongo_proxy", <collection>, <ListValue of ops>)` + a clear-per-pass discipline — the Bucket `Reset()`/`Set` surface suffices). Observability: zero cross-side surface (connection-level metadata has no in-repo consumer and is invisible to the differential) → unit-test proof + coverage note (D-P11). No retroactive zookeeper pickup obligation (AMEND-B11).

### 11.10 D29-10 — cx_destroy direction + drain posture

`onEvent` (`proxy.cc:355-376`): `RemoteClose` + non-empty active-query list → `cx_destroy_remote_with_active_rq`; `LocalClose` + non-empty list → `cx_destroy_local_with_active_rq`. Direction comes from the `Network::ConnectionEvent` enum delivered by the connection layer — the filter does not infer it. **As-built gap (AMEND-B12):** envoy-go's `ReadFilter.OnDestroy()` carries no close-direction/reason; the framework records close TYPE (FlushWrite etc.) but not WHO closed. D-P4 (29.2 SPEC): anticipated a minimal close-direction accessor on the connection/callbacks surface (threading the direction the chain runtime already observes), else a coverage boundary.

**Drain** (`proxy.cc:254-271, 290-292`): on a correlated reply, when the active-query list becomes EMPTY + `drain_decision_.drainClose(DrainDirection::All)` + runtime `drain_close_enabled` → `cx_drain_close` +1 → a ZERO-ms timer → `connection().close(FlushWrite)`. The close-between-operations semantics (never mid-query). envoy-go 29.3: integrate with the phase-08.2 drain machinery (the listener drain signal maps to `drainClose() == true`); the exact integration + the fixture-vs-unit-test proof posture is a 29.3 SPEC refinement.

### 11.11 D29-11 — per-sub-phase envelope (re-estimated against the empirical findings)

**29.1**: BSON parser (14 types + doc walk, ~250-350 LoC) + wire codec request side (header + 5 request opcodes, ~250-350) + config parse + commands set + FaultDelay validation (~120-180) + the 23-stat roster + dynamic-name helpers (~100-150) + filter glue + active-query list (~120-180) + builtins/bootstrap/name.go tag-extractor arm (~80-120) + fixtures `0049`/`0050` drivers (~500-700) + the 39th fuzzer (~60) ≈ **~1480-2090 total LoC of which ~920-1330 production**, ~14-19 tasks — fits the gate (production-LoC basis, the 26.x accounting precedent). **29.2**: OP_REPLY/OP_COMMANDREPLY decode (~120-180) + correlation + mutex + gauge (~120-180) + cx_destroy + close-direction seam (~80-150) + metadata emission (~60-100) + filter glue (~50) + fixture `0051` driver (~450-600) + fuzzer extension (~40) ≈ **~470-660 production LoC**, ~10-13 tasks — fits. **29.3**: the seam extension (active async resume + sync + post-handoff honoring + tests, ~200-350) + fault delay (~100-150) + access log formatter + sink seam (~120-200) + drain (~50-80) + fixture `0052` driver (~400-550) + rollup docs ≈ **~470-780 production LoC**, ~12-16 tasks — fits.

### 11.12 D29-12 — fuzzer envelope

The 39th fuzzer `FuzzMongoDecode` lands at 29.1: random bytes through the wire+BSON decoder asserting no-panic + no-chain-buffer-mutation + sniffing-off-on-error idempotence. Because mongo's decoder is direction-agnostic (a single opcode dispatch serves both directions — unlike zookeeper's two distinct framings), the anticipated 29.2 disposition is EXTENDING the 39th fuzzer's reach to the response opcodes rather than adding a 40th (D-P6; the 29.2 SPEC decides). Anticipated final fuzzer count: **39** (40 only if D-P6 resolves to a separate fuzzer). Fuzzer count recipe re-confirmed at each IMPL Task 1: `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l` = 38 at master tip.

---

## 12. SPEC-time D-questions for sub-phase SPEC / PLAN resolution

- **D-P1 (fixed-roster creation posture).** Eager-at-config-parse (freeze-after-boot friendly; boot-window departure recorded) vs first-connection creation (upstream parity). **Resolution at:** 29.1 SPEC. Anticipated: eager-at-config-parse; all fixture assertions post-first-connection make the difference unobservable.
- **D-P2 (dynamic-stat live shapes).** The `cmd.*`/`collection.*` names + their Prometheus form could not be live-probed (§11.1 caveat). **Resolution at:** 29.1 SPEC (re-probe via a working backend harness, or pin from upstream's tag-extraction source). Anticipated: the `envoy_mongo_prefix` label hoist applies uniformly; dynamic segments stay in the metric name.
- **D-P3 (name.go arm validation posture).** Shape-based detection (`mongo.` + dot-free prefix segment) vs an allowlist. **Resolution at:** 29.1 SPEC. Anticipated: shape-based (dynamic names make an allowlist impossible — the zookeeper D-P8 precedent).
- **D-P4 (close-direction seam).** AMEND-B12: a minimal close-direction accessor for `cx_destroy_local/remote_with_active_rq` vs a coverage boundary. **Resolution at:** 29.2 SPEC. Anticipated: a small accessor threading the direction the chain runtime already observes (no `TerminalFilter`/fake churn — the ADR-0219 no-ripple discipline).
- **D-P5 (delay-PGV fixture arms + header_delay posture).** Do the AMEND-B9 reject arms gain `0050` fixture arms (at 29.1 or 29.3), and what does a `header_delay`-configured mongo filter do (parse-accept-no-delay anticipated). **Resolution at:** 29.3 SPEC (arms unit-tested at 29.1 regardless).
- **D-P6 (response-decode fuzzer).** Extend the 39th vs add a 40th. **Resolution at:** 29.2 SPEC. Anticipated: extend (single direction-agnostic decoder entry).
- **D-P7 (access-log formatter seam).** A pluggable formatter on `AsyncFileSink` vs a mongo-owned sink reusing the async-writer internals. **Resolution at:** 29.3 SPEC.
- **D-P8 (commands-list fixture arm).** The `0049` corpus exercises a non-default `commands` list + the unknown_command fallback (§8.1 arm 2). **Resolution at:** 29.1 SPEC (anticipated: yes — the AMEND-B7 semantics need a live proof).
- **D-P9 (gauge quiesced-point assertion design).** How `0051` asserts `op_query_active` deterministically (answered → 0; unanswered → 1) given async reply timing. **Resolution at:** 29.2 SPEC. Anticipated: the driver waits for the reply to round-trip (byte receipt) before scraping; the unanswered arm never sends a reply.
- **D-P10 (StatsAsserter gauge support).** Whether `fixture.StatsAsserter` + the Prometheus scrape path handle gauge values/TYPE lines or need a small extension. **Resolution at:** 29.2 SPEC.
- **D-P11 (dynamic-metadata proof surface).** Unit-test-only + coverage note (anticipated per AMEND-B11) vs any observable consumer. **Resolution at:** 29.2 SPEC.
- **D-P12 (halt-state synchronization design).** The exact mutex/atomic/channel shape for cross-goroutine `ContinueReading` + post-handoff hold-and-release in `readChainConn.Read` (block vs withhold). **Resolution at:** 29.3 SPEC (the ADR-0223 minimal-critical-section discipline is the prior art).

---

## 13. RATIFIED-PENDING-IMPL items

- **R1 (seam back-compat).** Never-halting chains (every existing filter + mongo-without-delay) and zero-write-filter chains are byte-identical after the 29.3 seam extension. Ratified by ALL existing fixtures (0000..0051) staying byte-exact green at 29.3 (the seam's regression gate) + the `0052` no-delay arm.
- **R2 (roster + scope parity).** The 23-stat roster under `mongo.<stat_prefix>.` matches upstream name-for-name (incl. `delays_injected` plural). Ratified by the `0049` post-connection roster assertion + a `TestStatRoster_MatchesUpstreamMacro` byte-stable test.
- **R3 (passthrough invariant).** mongoproxy NEVER mutates/drains the chain buffer, never closes the connection (except the 29.3 drain close, which is upstream parity), and only returns StopIteration while a fault-delay timer pends. Decode errors → sniffing off + passthrough continues. Ratified by the `0049` garbage/OP_MSG arms (passthrough proven byte-exact through tcp_proxy) + unit tests.
- **R4 (StatsAsserter liveness).** Every `0049`/`0051`/`0052` stat assertion is proven live via a recorded deliberate-break run with `-count=1` (the `reference_differential_asserter_dispatch` + `reference_differential_break_protocol_count1` disciplines).
- **R5 (correlation hand-off).** The 29.1 active-query list is written-but-unread until 29.2 (the 28.1→28.2 xid-structures precedent). Ratified at 29.2 by the `0051` correlation + gauge arms.
- **R6 (counts re-pinned).** Each sub-phase IMPL Task 1 re-pins: fuzzers 38 (→39 at 29.1), fixtures 50 (tail `0048`; →52 at 29.1, →53 at 29.2, →54 at 29.3), stat surface 337 (→360 at 29.1 creation), DECISIONS.md tail ADR-0226 (next-free ADR-0227) — against the live IMPL-session tip.
- **R7 (Prometheus parity).** envoy-go's `/stats/prometheus` mongo lines match reference Envoy's tag-extracted shape `envoy_mongo_<leaf>{envoy_mongo_prefix="<prefix>"}` incl. the gauge TYPE. Ratified by the `0049`/`0051` StatsAsserter scrapes comparing the flattened forms.
- **R8 (deterministic fault arms).** The `0052` arms are 100%-probability + fixed-delay; `delays_injected` parity is asserted; the delay DURATION is never compared (BOOTSTRAP §7.2 timing discipline).

---

## 14. Test surface

Per the §9 family precedent (unit + fuzz + differential + race), per sub-phase:

- **29.1**: Layer A unit tests at `internal/filter/network/mongoproxy/` (config parse incl. all PGV arms + the commands default/replace/remember semantics + alias normalization; BSON parse per type + throw-on-unknown + nested docs + malformed docs; wire decode per opcode + partial-frame reassembly + OP_MSG/Update/Delete throw arms + flag bits + query-shape heuristics + $cmd/$comment/$maxTimeMS extraction; the 23-stat roster table; dynamic-name construction; sniffing-off-on-error) + `internal/stats/` (the `mongo.` tag-extractor arm incl. dynamic-name flattening); Layer C the 39th fuzzer; Layer D fixtures `0049` + `0050` + the FULL back-compat suite (50 existing dirs); Layer E `-race -short` across touched packages. Per-task `gofmt -l` + `golangci-lint` on touched packages (`feedback_pertask_gofmt_lint`).
- **29.2**: unit tests for OP_REPLY/OP_COMMANDREPLY decode + reply flags + correlation (match/no-match/erase) + the gauge inc/dec lifecycle + the per-connection mutex (a `-race` concurrent test per the ADR-0223 `TestDecoderConcurrentRequestResponseRace` precedent) + cx_destroy direction + metadata emission shape; fixture `0051` + the full suite; race.
- **29.3**: unit tests for the seam (active async resume; cross-goroutine `-race` tests; post-handoff hold-and-release; never-halting equivalence) + fault-delay (percentage eval, arm/cancel, inc-at-arm) + the access-log formatter goldens + drain; fixture `0052` + the full suite (53 prior dirs — the seam regression gate) + race.
- **Six-gate checklist** (per phase-22/24/25/26/27/28): `go build` / `go vet` / `golangci-lint run` / `go test ./... -race -short` / the FULL differential suite byte-exact / h2spec 53/53 + proxy-wasm 10/10 re-run (asserted-unaffected — phase 29 touches no HTTP path). All outputs quoted into PROGRESS.md (run honestly).

---

## 15. Per-sub-phase split-gate confirmation (D29-11 → ADR-0045)

| Sub-phase | Production LoC | Tasks | ADR-0045 gate (~25t / ~1500 LoC) | Verdict |
|---|---|---|---|---|
| 29.1 | ~920-1330 | ~14-19 | fits | ✅ (the largest; the PLAN re-checks — the fixture drivers dominate) |
| 29.2 | ~470-660 | ~10-13 | fits | ✅ |
| 29.3 | ~470-780 | ~12-16 | fits | ✅ |

Each sub-phase is independently shippable + delivers value: 29.1 ships a request-side-observable mongo filter with live cross-side stat parity; 29.2 completes both-direction observability + the first mirrored gauge; 29.3 lands the seam with its consumer + the family rollup. The 3-way pre-split holds at parent-SPEC time.

---

## 16. Stage-close handoff

Per ADR-0004/0005 (autonomous adaptation): this SPEC is reviewed by the `spec-document-reviewer` subagent (≤3 iterations); on approval, STATE.md advances to lifecycle-state 2-for-29.1 with `next-skill = superpowers:writing-plans` scoped to the **29.1 SPEC** (per the per-sub-phase precedent — the next session authors the 29.1 sub-phase SPEC, not the 29.1 PLAN, mirroring 22.1/25.1/26.1/28.1). ROADMAP: parent row 29 STAYS `in-progress`; sub-rows 29.1/29.2/29.3 STAY `planned` (29.1 flips `planned → in-progress` at the **29.1 SPEC** commit, NOT at this parent SPEC — the 26.x/28.x precedent). The parent SPEC + the ADR-0224/0225/0226 §Context drafts are squash-merged to master + pushed; next-prompt.txt is rewritten for the 29.1-SPEC cold-start.

---

## Appendix A — Phase 29 ADR landing summary

- **ADR-0224** *(§Context drafted at this SPEC; body at 29.1 IMPL)* — the `mongo_proxy` filter, request side: TypeURL + 5-field config parse (+ the FaultDelay PGV arms); the in-house BSON parser (the 14-type upstream subset, throw-on-unknown); the little-endian wire decoder (the EXACTLY-7-opcode envelope; OP_MSG/Update/Delete → decoding_error); the 23-stat fixed roster under `mongo.<stat_prefix>.` (creation parity per D-P1); request-side increments + the dynamic cmd/collection/callsite counter families (commands-remembering semantics); the per-connection active-query list; the 8th built-in; the `mongo.` Prometheus TAG-EXTRACTOR arm (ADR-0218 shape); fixtures `0049`/`0050`; the 39th fuzzer.
- **ADR-0225** *(§Context drafted at this SPEC; body at 29.2 IMPL)* — the response side: OP_REPLY/OP_COMMANDREPLY decode in `OnWrite` (consumer #2 of ADR-0221); requestID↔responseTo correlation + the per-connection mutex (the ADR-0223 pattern); the `op_query_active` gauge (the project's first differentially-mirrored gauge); `cx_destroy_*` counters + the close-direction resolution (D-P4); dynamic-metadata emission (ADR-0217 Bucket third production write); fixture `0051`.
- **ADR-0226** *(§Context drafted at this SPEC; body at 29.3 IMPL)* — the async halt/resume seam extension (active asynchronous `ContinueReading` + cross-goroutine safety + post-handoff read-halt honoring; zero new exported API; never-halting chains byte-identical) + fault-delay injection (deterministic differential arms) + the mongo access log (timing-bearing format → unit-test + coverage boundary) + `cx_drain_close` + fixture `0052` + the parent-row-29 ROLLUP; CONSUMES the ADR-0221 §Consequences anticipated-halt-consumer allowance (refined to the READ side).
